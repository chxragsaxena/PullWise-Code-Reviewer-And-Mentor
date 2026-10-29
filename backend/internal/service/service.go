// Package service accepts pull request links, runs reviews in the background and serves results.
package service

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/rules"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

var (
	// ErrInvalidPRURL is returned when the link is not a GitHub pull request.
	ErrInvalidPRURL = github.ErrInvalidPRURL
	// ErrBusy is returned when too many reviews are already waiting or running.
	ErrBusy = errors.New("too many reviews in progress, try again shortly")
	// ErrShuttingDown is returned once the service has stopped accepting work.
	ErrShuttingDown = errors.New("service is shutting down")
)

// ReviewStore is what request handling needs from persistence.
type ReviewStore interface {
	CreateReview(ctx context.Context, owner, repo string, number int) (review.Review, error)
	GetReview(ctx context.Context, id string) (review.Review, error)
	ListReviews(ctx context.Context, limit int) ([]review.Review, error)
	GetFinding(ctx context.Context, reviewID, findingID string) (review.Finding, error)
	ReviewRules(ctx context.Context, reviewID string) ([]review.Rule, error)
}

// ReviewRecorder is what the background pipeline needs from persistence.
type ReviewRecorder interface {
	SetStatus(ctx context.Context, id string, status review.Status, message string) error
	SavePullRequest(ctx context.Context, id string, pr review.PullRequest) error
	CompleteReview(ctx context.Context, id string, findings []review.Finding, message string) error
	FailReview(ctx context.Context, id, code, message string) error
	ReviewRules(ctx context.Context, reviewID string) ([]review.Rule, error)
}

// PullRequestSource reads pull requests from GitHub.
type PullRequestSource interface {
	FetchPullRequest(ctx context.Context, ref github.PullRequestRef) (github.PullRequest, error)
	ListFiles(ctx context.Context, ref github.PullRequestRef) ([]github.ChangedFile, error)
	FileContent(ctx context.Context, ref github.PullRequestRef, path, sha string) ([]byte, error)
}

// Config tunes the service. Zero values get sensible defaults.
type Config struct {
	Skills    []skills.Skill
	Analyzers []review.StaticAnalyzer
	// AIProvider evaluates each skill's AI checks. Nil disables the AI pass.
	AIProvider ai.Provider
	// Knowledge gives the AI pass related code from the repository. Nil disables it.
	Knowledge Knowledge

	// MaxConcurrent is how many reviews run at once (default 2).
	MaxConcurrent int
	// MaxPending is how many reviews may queue before ErrBusy (default 50).
	MaxPending int
	// ReviewTimeout bounds one review from start to finish (default 5 minutes).
	ReviewTimeout time.Duration
	// MaxFilesToAnalyze caps source files downloaded per review (default 100).
	MaxFilesToAnalyze int
}

func (c Config) withDefaults() Config {
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 2
	}
	if c.MaxPending <= 0 {
		c.MaxPending = 50
	}
	if c.ReviewTimeout <= 0 {
		c.ReviewTimeout = 5 * time.Minute
	}
	if c.MaxFilesToAnalyze <= 0 {
		c.MaxFilesToAnalyze = 100
	}
	return c
}

// Service coordinates reviews. It is safe for concurrent use.
type Service struct {
	store      ReviewStore
	recorder   ReviewRecorder
	source     PullRequestSource
	broker     *Broker
	cfg        Config
	skills     map[string]skills.Skill
	aiProvider ai.Provider
	knowledge  Knowledge

	jobs    chan job // FIFO queue feeding the workers
	baseCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup // the workers

	mu       sync.Mutex // guards closed, reserved and sends on jobs
	closed   bool
	reserved int // queue slots held by Create calls mid-write
}

// job is one queued review.
type job struct {
	id  string
	ref github.PullRequestRef
	// token is the creating user's GitHub token; kept in memory only, never stored.
	token string
	// viewer is the creating user's id ("" if anonymous); it picks the knowledge index.
	viewer string
}

// New builds a Service and starts its workers. Call Close to stop them.
func New(store ReviewStore, recorder ReviewRecorder, source PullRequestSource, cfg Config) *Service {
	cfg = cfg.withDefaults()
	byID := make(map[string]skills.Skill, len(cfg.Skills))
	for _, sk := range cfg.Skills {
		byID[sk.ID] = sk
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		store:      store,
		recorder:   recorder,
		source:     source,
		broker:     NewBroker(),
		cfg:        cfg,
		skills:     byID,
		aiProvider: cfg.AIProvider,
		knowledge:  cfg.Knowledge,
		jobs:       make(chan job, cfg.MaxPending),
		baseCtx:    ctx,
		cancel:     cancel,
	}

	s.wg.Add(cfg.MaxConcurrent)
	for range cfg.MaxConcurrent {
		go s.worker()
	}
	return s
}

// worker processes queued reviews one at a time until the queue is closed.
func (s *Service) worker() {
	defer s.wg.Done()
	for j := range s.jobs {
		s.run(j)
	}
}

// Create validates the link, records a queued review and starts it in the background.
func (s *Service) Create(ctx context.Context, prURL string) (review.Review, error) {
	ref, err := github.ParsePRURL(prURL)
	if err != nil {
		return review.Review{}, err
	}

	// Reserve a queue slot first so a refused request leaves no review behind.
	s.mu.Lock()
	switch {
	case s.closed:
		s.mu.Unlock()
		return review.Review{}, ErrShuttingDown
	case len(s.jobs)+s.reserved >= cap(s.jobs):
		s.mu.Unlock()
		return review.Review{}, ErrBusy
	}
	s.reserved++
	s.mu.Unlock()

	// The database write happens outside the lock so requests do not serialize.
	rv, err := s.store.CreateReview(ctx, ref.Owner, ref.Repo, ref.Number)

	// Close closes the queue under this lock, so we never send on a closed channel.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reserved--
	if err != nil {
		return review.Review{}, err
	}
	if s.closed {
		// Shutdown began while the record was being written; nobody will run it.
		s.failUnstarted(rv.ID)
		return review.Review{}, ErrShuttingDown
	}
	// Room is guaranteed by the reservation.
	s.jobs <- job{id: rv.ID, ref: ref, token: github.RequestToken(ctx), viewer: review.ViewerFrom(ctx)}
	return rv, nil
}

// failUnstarted marks a review that will never be processed as failed.
func (s *Service) failUnstarted(id string) {
	ctx, cancel := recordContext(s.baseCtx)
	defer cancel()
	if err := s.recorder.FailReview(ctx, id, codeInterrupted, "The server shut down before this review started."); err != nil {
		slog.Error("could not record unstarted review", "review", id, "error", err)
	}
}

// Get returns a review with its findings.
func (s *Service) Get(ctx context.Context, id string) (review.Review, error) {
	rv, err := s.store.GetReview(ctx, id)
	if err != nil {
		return review.Review{}, err
	}
	custom := s.customSkill(ctx, id, rv.Findings)
	for i := range rv.Findings {
		rv.Findings[i] = s.enrichWith(rv.Findings[i], custom)
	}
	return rv, nil
}

// List returns recent reviews, newest first. limit is clamped to 1..50 (default 20).
func (s *Service) List(ctx context.Context, limit int) ([]review.Review, error) {
	switch {
	case limit <= 0:
		limit = 20
	case limit > 50:
		limit = 50
	}
	return s.store.ListReviews(ctx, limit)
}

// Lesson is teaching material attached to a finding.
type Lesson struct {
	ID       string
	Markdown string
}

// FindingDetail is a finding together with its lesson, if it has one.
type FindingDetail struct {
	Finding review.Finding
	Lesson  *Lesson
}

// GetFinding returns one finding with its lesson text.
func (s *Service) GetFinding(ctx context.Context, reviewID, findingID string) (FindingDetail, error) {
	f, err := s.store.GetFinding(ctx, reviewID, findingID)
	if err != nil {
		return FindingDetail{}, err
	}
	custom := s.customSkill(ctx, reviewID, []review.Finding{f})
	f = s.enrichWith(f, custom)

	detail := FindingDetail{Finding: f}
	if f.Lesson != "" {
		if sk, ok := s.skillByID(f.SkillID, custom); ok {
			if md, err := sk.Lesson(f.Lesson); err != nil {
				slog.Warn("lesson unavailable", "skill", f.SkillID, "lesson", f.Lesson, "error", err)
			} else {
				detail.Lesson = &Lesson{ID: f.Lesson, Markdown: md}
			}
		}
	}
	return detail, nil
}

// customSkill returns the review's own-standards skill if any finding came from it.
func (s *Service) customSkill(ctx context.Context, reviewID string, findings []review.Finding) *skills.Skill {
	if !slices.ContainsFunc(findings, func(f review.Finding) bool { return f.SkillID == rules.SkillID }) {
		return nil
	}
	rs, err := s.store.ReviewRules(ctx, reviewID)
	if err != nil {
		slog.Warn("could not read the review's rules", "review", reviewID, "error", err)
		return nil
	}
	if sk, ok := rules.Skill(rs); ok {
		return &sk
	}
	return nil
}

// skillByID finds a built-in skill, or the review's custom one.
func (s *Service) skillByID(id string, custom *skills.Skill) (skills.Skill, bool) {
	if sk, ok := s.skills[id]; ok {
		return sk, true
	}
	if custom != nil && custom.ID == id {
		return *custom, true
	}
	return skills.Skill{}, false
}

func (s *Service) enrich(f review.Finding) review.Finding { return s.enrichWith(f, nil) }

// enrichWith fills display names from skill definitions, which the database does not store.
func (s *Service) enrichWith(f review.Finding, custom *skills.Skill) review.Finding {
	if sk, ok := s.skillByID(f.SkillID, custom); ok {
		f.SkillName = sk.Name
		if c, ok := sk.Check(f.CheckID); ok {
			f.CheckTitle = c.Title
		}
	}
	if f.SkillName == "" {
		f.SkillName = f.SkillID
	}
	if f.CheckTitle == "" {
		f.CheckTitle = f.CheckID
	}
	return f
}

// Stream is a live view of one review's status changes.
type Stream struct {
	// Current is the status at the moment of subscribing.
	Current review.Event
	// Events delivers later changes; nil when Current is terminal, closed if the subscriber lags.
	Events <-chan review.Event
	// Close stops the stream. It is always safe to call.
	Close func()
}

// Watch subscribes before reading the current status, so no change is missed (one may repeat).
func (s *Service) Watch(ctx context.Context, id string) (*Stream, error) {
	events, cancel := s.broker.Subscribe(id)

	rv, err := s.store.GetReview(ctx, id)
	if err != nil {
		cancel()
		return nil, err
	}

	current := review.Event{Status: rv.Status, Message: rv.StatusMessage, At: rv.StatusAt}
	if rv.Status.Terminal() {
		cancel()
		return &Stream{Current: current, Close: func() {}}, nil
	}
	return &Stream{Current: current, Events: events, Close: cancel}, nil
}

// Close stops accepting reviews and waits for in-flight ones; if ctx ends first it cancels them.
func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.jobs) // workers drain what is queued, then exit
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.cancel()
		return nil
	case <-ctx.Done():
		s.cancel() // running reviews notice, record "interrupted" and return
		<-done
		return ctx.Err()
	}
}
