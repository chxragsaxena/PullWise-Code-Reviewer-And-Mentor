// Package httpapi exposes the REST API over net/http.
package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/service"
)

// ReviewService is what the handlers need from the review service.
type ReviewService interface {
	Create(ctx context.Context, prURL string) (review.Review, error)
	Get(ctx context.Context, id string) (review.Review, error)
	List(ctx context.Context, limit int) ([]review.Review, error)
	GetFinding(ctx context.Context, reviewID, findingID string) (service.FindingDetail, error)
	Watch(ctx context.Context, id string) (*service.Stream, error)
}

// API serves the REST endpoints described in docs/api.md.
type API struct {
	svc       ReviewService
	ready     func(context.Context) error
	heartbeat time.Duration
	create    *limiter // nil when rate limiting is off

	// auth is nil when GitHub login is not configured — every /auth/*
	// handler then degrades to "not logged in" or 404, matching the
	// project's pattern for optional features (GITHUB_TOKEN, GROQ_API_KEY).
	auth          AuthService
	frontendURL   string
	secureCookies bool

	done     chan struct{} // closed by BeginShutdown to end open event streams
	doneOnce sync.Once
}

// Option configures an API.
type Option func(*API)

// WithReadiness sets the check behind GET /readyz, typically a database ping.
func WithReadiness(check func(context.Context) error) Option {
	return func(a *API) { a.ready = check }
}

// WithHeartbeat sets how often an idle event stream sends a keep-alive comment
// (default 15s), so proxies do not close it.
func WithHeartbeat(d time.Duration) Option { return func(a *API) { a.heartbeat = d } }

// WithCreateRateLimit limits POST /api/reviews per client address. perMinute <= 0
// turns the limit off. The client address is the connection's remote address; behind
// a reverse proxy every client shares the proxy's address.
func WithCreateRateLimit(perMinute, burst int) Option {
	return func(a *API) {
		if perMinute > 0 && burst > 0 {
			a.create = newLimiter(perMinute, burst, time.Now)
		} else {
			a.create = nil
		}
	}
}

// WithAuth enables GitHub login. frontendURL is where the browser is sent
// after a successful login; secureCookies marks session/state cookies
// Secure, which requires HTTPS — leave this off for local HTTP development.
func WithAuth(svc AuthService, frontendURL string, secureCookies bool) Option {
	return func(a *API) { a.auth, a.frontendURL, a.secureCookies = svc, frontendURL, secureCookies }
}

// New builds an API over a review service.
func New(svc ReviewService, opts ...Option) *API {
	a := &API{svc: svc, heartbeat: 15 * time.Second, done: make(chan struct{})}
	WithCreateRateLimit(20, 10)(a)
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// BeginShutdown ends open event streams so http.Server.Shutdown is not held up
// by them. Register it with http.Server.RegisterOnShutdown.
func (a *API) BeginShutdown() { a.doneOnce.Do(func() { close(a.done) }) }

// Handler returns the router with its middleware.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/healthz", methods{http.MethodGet: handleHealth})
	mux.Handle("/readyz", methods{http.MethodGet: a.handleReady})

	mux.Handle("/api/reviews", methods{
		http.MethodPost: a.createReview,
		http.MethodGet:  a.listReviews,
	})
	mux.Handle("/api/reviews/{id}", methods{http.MethodGet: a.getReview})
	mux.Handle("/api/reviews/{id}/events", methods{http.MethodGet: a.streamEvents})
	mux.Handle("/api/reviews/{id}/findings/{findingId}", methods{http.MethodGet: a.getFinding})

	mux.Handle("/auth/github/login", methods{http.MethodGet: a.handleLogin})
	mux.Handle("/auth/github/callback", methods{http.MethodGet: a.handleCallback})
	mux.Handle("/auth/me", methods{http.MethodGet: a.handleMe})
	mux.Handle("/auth/logout", methods{http.MethodPost: a.handleLogout})

	mux.HandleFunc("/", notFound)

	// recoverer is outermost so a panic in any handler or middleware is caught.
	return recoverer(requestLogger(secureHeaders(mux)))
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleReady(w http.ResponseWriter, r *http.Request) {
	if a.ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.ready(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "not_ready", "The service is not ready.")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
