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
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required (see .env.example)"))
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
