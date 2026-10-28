package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/auth"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/service"
)

type seenCtx struct{ viewer, token string }

func capture(ctx context.Context) seenCtx {
	return seenCtx{viewer: review.ViewerFrom(ctx), token: github.RequestToken(ctx)}
}

func signedIn(fa *fakeAuth) *fakeAuth {
	fa.usersByToken = map[string]auth.User{"tok": {ID: "42", GitHubLogin: "alice"}}
	return fa
}

func requestAs(h http.Handler, method, path, body string, withSession bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if withSession {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "tok"})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateReviewCarriesTheViewerAndTheirGitHubToken(t *testing.T) {
	tests := []struct {
		name        string
		withSession bool
		githubToken string
		githubErr   error
		want        seenCtx
	}{
		{"anonymous", false, "ignored", nil, seenCtx{}},
		{"signed in with a usable token", true, "gho_user", nil, seenCtx{viewer: "42", token: "gho_user"}},
		{"signed in but the token expired: still the owner, no token", true, "", auth.ErrTokenExpired, seenCtx{viewer: "42"}},
		{"signed in but the token can't be loaded: still the owner, no token", true, "", errors.New("db down"), seenCtx{viewer: "42"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got seenCtx
			svc := fakeService{create: func(ctx context.Context, _ string) (review.Review, error) {
				got = capture(ctx)
				return sampleReview(), nil
			}}
			fa := signedIn(&fakeAuth{githubToken: tt.githubToken, githubErr: tt.githubErr})
			h := New(svc, WithCreateRateLimit(0, 0), WithAuth(fa, "http://localhost:5173", false)).Handler()

			rec := requestAs(h, http.MethodPost, "/api/reviews", `{"prUrl":"https://github.com/o/r/pull/3"}`, tt.withSession)

			if rec.Code != http.StatusAccepted {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
			}
			if got != tt.want {
				t.Fatalf("service saw %+v, want %+v", got, tt.want)
			}
			if strings.Contains(rec.Body.String(), "gho_user") {
				t.Fatal("the GitHub token leaked into the response")
			}
		})
	}
}

func TestEveryReadEndpointIsAskedOnBehalfOfTheViewer(t *testing.T) {
	var seen []seenCtx
	note := func(ctx context.Context) { seen = append(seen, capture(ctx)) }
	svc := fakeService{
		get: func(ctx context.Context, _ string) (review.Review, error) { note(ctx); return sampleReview(), nil },
		list: func(ctx context.Context, _ int) ([]review.Review, error) {
			note(ctx)
			return nil, nil
		},
		getFinding: func(ctx context.Context, _, _ string) (service.FindingDetail, error) {
			note(ctx)
			return service.FindingDetail{}, review.ErrFindingNotFound
		},
		watch: func(ctx context.Context, _ string) (*service.Stream, error) {
			note(ctx)
			return nil, review.ErrReviewNotFound
		},
	}
	h := New(svc, WithCreateRateLimit(0, 0), WithAuth(signedIn(&fakeAuth{}), "http://localhost:5173", false)).Handler()

	for _, path := range []string{"/api/reviews", "/api/reviews/7", "/api/reviews/7/findings/1", "/api/reviews/7/events"} {
		seen = nil
		requestAs(h, http.MethodGet, path, "", true)
		if len(seen) != 1 || seen[0].viewer != "42" {
			t.Errorf("GET %s: service saw %+v, want one call as viewer 42", path, seen)
		}
		if len(seen) == 1 && seen[0].token != "" {
			t.Errorf("GET %s: a read must never carry the GitHub token", path)
		}

		seen = nil
		requestAs(h, http.MethodGet, path, "", false)
		if len(seen) != 1 || seen[0].viewer != "" {
			t.Errorf("anonymous GET %s: service saw %+v, want an anonymous call", path, seen)
		}
	}
}
