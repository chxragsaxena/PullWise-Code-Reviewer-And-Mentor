// Command server runs the PullWise REST API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/analyzer/goast"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/analyzer/tsast"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/config"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/httpapi"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/service"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/store"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/migrations"
)

const (
	httpShutdownTimeout    = 10 * time.Second
	reviewsShutdownTimeout = 30 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Real environment variables win over .env; the file is only a convenience.
	// It is looked up here and in the parent, because the repo's .env sits beside
	// backend/ when running from there.
	if loaded, err := config.LoadDotEnv(".env", "../.env"); err != nil {
		return fmt.Errorf("load .env: %w", err)
	} else if len(loaded) > 0 {
		slog.Info("loaded environment file", "paths", loaded)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	loadedSkills, err := skills.Load(os.DirFS(cfg.SkillsDir))
	if err != nil {
		return fmt.Errorf("load skills from %q: %w", cfg.SkillsDir, err)
	}
	slog.Info("skills loaded", "count", len(loadedSkills))

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := store.Migrate(ctx, db, migrations.Files); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	st := store.New(db)
	if err := st.SyncSkills(ctx, loadedSkills); err != nil {
		return err
	}
	// Reviews still in progress belong to a process that no longer exists. This
	// assumes a single server instance; with several, do it only on one of them.
	if n, err := st.FailInterrupted(ctx); err != nil {
		return err
	} else if n > 0 {
		slog.Warn("marked reviews interrupted by the previous shutdown as failed", "count", n)
	}

	var opts []github.Option
	if cfg.GitHubToken != "" {
		opts = append(opts, github.WithToken(cfg.GitHubToken))
	} else {
		slog.Warn("GITHUB_TOKEN is not set: GitHub allows only 60 API requests per hour without one")
	}

	// go/ast covers the Go skill's deterministic checks; tree-sitter covers
	// TypeScript's (Semgrep, for pattern-based checks, isn't built yet).
	analyzers := []review.StaticAnalyzer{
		goast.New(goast.Config{ComplexityThreshold: complexityThreshold(loadedSkills, "go")}),
		tsast.New(tsast.Config{ComplexityThreshold: complexityThreshold(loadedSkills, "typescript")}),
	}

	// The AI pass (kind: ai checks) runs on Groq, free and fast; see
	// internal/ai/groq.go for why. Nil disables it entirely if no key is set.
	// The system prompt lives in skills/system_prompt.md, editable like any
	// other skill/lesson file, with a built-in fallback if it's ever removed.
	systemPrompt, err := ai.LoadSystemPrompt(os.DirFS(cfg.SkillsDir))
	if err != nil {
		return fmt.Errorf("load system prompt: %w", err)
	}

	var aiProvider ai.Provider
	if cfg.GroqAPIKey != "" {
		aiProvider = ai.NewGroqProvider(cfg.GroqAPIKey, ai.WithGroqSystemPrompt(systemPrompt))
	} else {
		slog.Warn("GROQ_API_KEY is not set: the AI review pass is disabled, reviews will be deterministic-only")
	}

	svc := service.New(st, st, github.NewClient(opts...), service.Config{
		Skills:        loadedSkills,
		Analyzers:     analyzers,
		AIProvider:    aiProvider,
		MaxConcurrent: cfg.MaxConcurrentReviews,
		MaxPending:    cfg.MaxQueuedReviews,
		ReviewTimeout: cfg.ReviewTimeout,
	})

	api := httpapi.New(svc,
		httpapi.WithReadiness(db.PingContext),
		httpapi.WithCreateRateLimit(cfg.CreateRatePerMinute, cfg.CreateBurst),
	)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		// No WriteTimeout: event streams stay open for as long as a review runs.
	}
	srv.RegisterOnShutdown(api.BeginShutdown)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			closeService(svc)
			return fmt.Errorf("listen: %w", err)
		}
		return nil
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	// Stop taking requests first, then let running reviews finish, then the
	// deferred db.Close runs last so reviews can still save their results.
	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancelHTTP()
	if err := srv.Shutdown(httpCtx); err != nil {
		slog.Error("http shutdown", "error", err)
	}
	closeService(svc)
	return nil
}

// complexityThreshold reads a skill's high-complexity threshold from its
// skill.yaml, so the analyzer and the skill definition cannot drift apart.
// It returns 0 (the analyzer's own default) if the skill or check is missing.
func complexityThreshold(loaded []skills.Skill, skillID string) int {
	for _, sk := range loaded {
		if sk.ID != skillID {
			continue
		}
		if check, ok := sk.Check("high-complexity"); ok {
			return check.Threshold
		}
	}
	return 0
}

// closeService waits for running reviews, then interrupts whatever is left.
func closeService(svc *service.Service) {
	ctx, cancel := context.WithTimeout(context.Background(), reviewsShutdownTimeout)
	defer cancel()
	if err := svc.Close(ctx); err != nil {
		slog.Warn("some reviews were interrupted by shutdown", "error", err)
	}
}
