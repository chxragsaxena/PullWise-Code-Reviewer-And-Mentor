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
	"slices"
	"syscall"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/analyzer/goast"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/analyzer/tsast"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ask"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/auth"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/config"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/httpapi"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/knowledge"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/rules"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/service"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/store"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/tlsconfig"
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
	// The placeholder is the skill row that findings from users' own rules point at.
	if err := st.SyncSkills(ctx, append(slices.Clone(loadedSkills), rules.Placeholder())); err != nil {
		return err
	}
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

	analyzers := []review.StaticAnalyzer{
		goast.New(goast.Config{ComplexityThreshold: complexityThreshold(loadedSkills, "go")}),
		tsast.New(tsast.Config{ComplexityThreshold: complexityThreshold(loadedSkills, "typescript")}),
	}

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

	var kb service.Knowledge
	if cfg.KnowledgeURL != "" {
		client, err := newKnowledgeClient(cfg)
		if err != nil {
			return err
		}
		kb = client
		slog.Info("repository knowledge base enabled", "url", cfg.KnowledgeURL, "mutual_tls", cfg.KnowledgeTLSCertFile != "")
	} else {
		slog.Warn("KNOWLEDGE_URL is not set: the AI pass will not see code beyond the diff")
	}

	ghClient := github.NewClient(opts...)
	svc := service.New(st, st, ghClient, service.Config{
		Skills:        loadedSkills,
		Analyzers:     analyzers,
		AIProvider:    aiProvider,
		Knowledge:     kb,
		MaxConcurrent: cfg.MaxConcurrentReviews,
		MaxPending:    cfg.MaxQueuedReviews,
		ReviewTimeout: cfg.ReviewTimeout,
	})

	// Questions need both the knowledge base and a chat-capable AI; otherwise the endpoint reports "unavailable".
	chatter, _ := aiProvider.(ai.Chatter)
	askPrompt, err := ai.LoadAskPrompt(os.DirFS(cfg.SkillsDir))
	if err != nil {
		return fmt.Errorf("load ask prompt: %w", err)
	}
	askSvc := ask.New(ask.Config{Knowledge: kb, Repos: ghClient, Reviews: svc, Chat: chatter, Prompt: askPrompt})

	apiOpts := []httpapi.Option{
		httpapi.WithRules(rules.NewService(st)),
		httpapi.WithAsk(askSvc),
		httpapi.WithAskRateLimit(cfg.AskRatePerMinute, cfg.AskBurst),
		httpapi.WithReadiness(db.PingContext),
		httpapi.WithCreateRateLimit(cfg.CreateRatePerMinute, cfg.CreateBurst),
	}

	if cfg.GitHubOAuthClientID != "" {
		key, err := auth.DecodeKey(cfg.SessionSecret)
		if err != nil {
			return fmt.Errorf("decode SESSION_SECRET: %w", err)
		}
		encryptor, err := auth.NewEncryptor(key)
		if err != nil {
			return fmt.Errorf("build token encryptor: %w", err)
		}
		oauth := auth.NewOAuth(cfg.GitHubOAuthClientID, cfg.GitHubOAuthClientSecret, cfg.GitHubOAuthRedirectURL)
		authSvc := auth.NewService(oauth, encryptor, st)
		apiOpts = append(apiOpts, httpapi.WithAuth(authSvc, cfg.FrontendURL, cfg.SecureCookies))

		if n, err := st.DeleteExpiredSessions(ctx); err != nil {
			slog.Warn("delete expired sessions", "error", err)
		} else if n > 0 {
			slog.Info("deleted expired sessions", "count", n)
		}
	} else {
		slog.Warn("GITHUB_OAUTH_CLIENT_ID is not set: GitHub login is disabled")
	}

	api := httpapi.New(svc, apiOpts...)
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

	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancelHTTP()
	if err := srv.Shutdown(httpCtx); err != nil {
		slog.Error("http shutdown", "error", err)
	}
	closeService(svc)
	return nil
}

func newKnowledgeClient(cfg config.Config) (*knowledge.Client, error) {
	var opts []knowledge.Option
	if cfg.KnowledgeTLSCAFile != "" || cfg.KnowledgeTLSCertFile != "" {
		tlsCfg, err := tlsconfig.Client(cfg.KnowledgeTLSCertFile, cfg.KnowledgeTLSKeyFile, cfg.KnowledgeTLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("knowledge service TLS: %w", err)
		}
		opts = append(opts, knowledge.WithTLS(tlsCfg))
	}
	client, err := knowledge.New(cfg.KnowledgeURL, []byte(cfg.KnowledgeJWTSecret), cfg.KnowledgeJWTIssuer, cfg.KnowledgeJWTAudience, opts...)
	if err != nil {
		return nil, fmt.Errorf("knowledge service client: %w", err)
	}
	return client, nil
}

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
