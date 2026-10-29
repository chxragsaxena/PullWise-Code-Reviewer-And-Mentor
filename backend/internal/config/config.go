// Package config reads server settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds process-wide settings.
type Config struct {
	// Addr is the address the HTTP server listens on.
	Addr string
	// SkillsDir is the directory containing skill folders.
	SkillsDir string
	// DatabaseURL is the PostgreSQL connection string (required).
	DatabaseURL string
	// GitHubToken is optional; it raises GitHub's API rate limit.
	GitHubToken string
	// GroqAPIKey enables the AI review pass (kind: ai checks) via Groq. Empty
	// disables it — reviews stay deterministic-only, exactly as before it existed.
	GroqAPIKey string

	// GitHub OAuth (login). Empty ClientID disables login entirely — every
	// /auth/* route then degrades to "not configured" / "not logged in".
	GitHubOAuthClientID     string
	GitHubOAuthClientSecret string
	GitHubOAuthRedirectURL  string
	// SessionSecret is the base64-encoded AES-256 key used to encrypt stored
	// GitHub tokens. Generate 32 random bytes and base64-encode them.
	SessionSecret string
	// FrontendURL is where the browser is sent after a successful login.
	FrontendURL string
	// SecureCookies marks session/state cookies Secure (HTTPS-only). Leave
	// false for local HTTP development.
	SecureCookies bool

	// The knowledge service (separate repo: pullwise-knowledge) gives the AI
	// review related code from the rest of the repository. Empty URL disables it.
	KnowledgeURL string
	// KnowledgeJWTSecret signs the tokens sent to it; it must equal the secret
	// the knowledge service verifies with.
	KnowledgeJWTSecret   string
	KnowledgeJWTIssuer   string
	KnowledgeJWTAudience string
	// Optional TLS: the CA that signed the service's certificate, and this
	// service's own certificate and key (for mutual TLS).
	KnowledgeTLSCAFile   string
	KnowledgeTLSCertFile string
	KnowledgeTLSKeyFile  string

	// MaxConcurrentReviews is how many reviews are processed at once.
	MaxConcurrentReviews int
	// MaxQueuedReviews is how many reviews may wait for a free worker.
	MaxQueuedReviews int
	// ReviewTimeout bounds a single review.
	ReviewTimeout time.Duration

	// CreateRatePerMinute and CreateBurst limit POST /api/reviews per client.
	// A rate of 0 turns the limit off.
	CreateRatePerMinute int
	CreateBurst         int
}

// Load reads configuration from environment variables, applying defaults, and
// reports every problem it finds at once rather than one per run.
func Load() (Config, error) {
	var errs []error
	c := Config{
		Addr:        getenv("ADDR", ":8080"),
		SkillsDir:   getenv("SKILLS_DIR", "skills"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		GitHubToken: os.Getenv("GITHUB_TOKEN"),
		GroqAPIKey:  os.Getenv("GROQ_API_KEY"),

		GitHubOAuthClientID:     os.Getenv("GITHUB_OAUTH_CLIENT_ID"),
		GitHubOAuthClientSecret: os.Getenv("GITHUB_OAUTH_CLIENT_SECRET"),
		GitHubOAuthRedirectURL:  getenv("GITHUB_OAUTH_REDIRECT_URL", "http://localhost:8080/auth/github/callback"),
		SessionSecret:           os.Getenv("SESSION_SECRET"),
		FrontendURL:             getenv("FRONTEND_URL", "http://localhost:5173"),
		SecureCookies:           os.Getenv("SECURE_COOKIES") == "true",

		KnowledgeURL:         os.Getenv("KNOWLEDGE_URL"),
		KnowledgeJWTSecret:   os.Getenv("KNOWLEDGE_JWT_SECRET"),
		KnowledgeJWTIssuer:   getenv("KNOWLEDGE_JWT_ISSUER", "pullwise"),
		KnowledgeJWTAudience: getenv("KNOWLEDGE_JWT_AUDIENCE", "pullwise-knowledge"),
		KnowledgeTLSCAFile:   os.Getenv("KNOWLEDGE_TLS_CA_FILE"),
		KnowledgeTLSCertFile: os.Getenv("KNOWLEDGE_TLS_CERT_FILE"),
		KnowledgeTLSKeyFile:  os.Getenv("KNOWLEDGE_TLS_KEY_FILE"),
	}
	if c.KnowledgeURL != "" && len(c.KnowledgeJWTSecret) < 32 {
		errs = append(errs, errors.New("KNOWLEDGE_JWT_SECRET (at least 32 bytes) is required when KNOWLEDGE_URL is set"))
	}
	if (c.KnowledgeTLSCertFile == "") != (c.KnowledgeTLSKeyFile == "") {
		errs = append(errs, errors.New("KNOWLEDGE_TLS_CERT_FILE and KNOWLEDGE_TLS_KEY_FILE must be set together"))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required (see .env.example)"))
	}
	if c.GitHubOAuthClientID != "" && c.SessionSecret == "" {
		errs = append(errs, errors.New("SESSION_SECRET is required when GITHUB_OAUTH_CLIENT_ID is set"))
	}

	c.MaxConcurrentReviews = intEnv("MAX_CONCURRENT_REVIEWS", 2, 1, &errs)
	c.MaxQueuedReviews = intEnv("MAX_QUEUED_REVIEWS", 50, 1, &errs)
	c.CreateRatePerMinute = intEnv("CREATE_RATE_PER_MINUTE", 20, 0, &errs)
	c.CreateBurst = intEnv("CREATE_BURST", 10, 1, &errs)
	c.ReviewTimeout = durationEnv("REVIEW_TIMEOUT", 5*time.Minute, &errs)

	return c, errors.Join(errs...)
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback, min int, errs *[]error) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min {
		*errs = append(*errs, fmt.Errorf("%s must be a whole number of at least %d, got %q", key, min, raw))
		return fallback
	}
	return n
}

func durationEnv(key string, fallback time.Duration, errs *[]error) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		*errs = append(*errs, fmt.Errorf("%s must be a positive duration like 5m or 90s, got %q", key, raw))
		return fallback
	}
	return d
}
