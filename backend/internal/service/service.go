// Package service runs reviews: it accepts a pull request link, records a
// review and processes it in the background, and serves what was found.
package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
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
}

// ReviewRecorder is what the background pipeline needs from persistence.
type ReviewRecorder interface {
	SetStatus(ctx context.Context, id string, status review.Status, message string) error
	SavePullRequest(ctx context.Context, id string, pr review.PullRequest) error
	CompleteReview(ctx context.Context, id string, findings []review.Finding, message string) error
	FailReview(ctx context.Context, id, code, message string) error
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
	// AIProvider evaluates each selected skill's kind: ai checks. Nil disables
	// the AI pass entirely: reviews go straight from analyzing to completed,
	// exactly as before this existed.
	AIProvider ai.Provider

	// MaxConcurrent is how many reviews run at once, i.e. the number of workers
	// (default 2).
	MaxConcurrent int
	// MaxPending is how many reviews may wait in the queue for a free worker
	// before new ones are refused with ErrBusy (default 50).
	MaxPending int
	// ReviewTimeout bounds one review from start to finish (default 5 minutes).
	ReviewTimeout time.Duration
	// MaxFilesToAnalyze caps how many source files are downloaded and analyzed
	// per review (default 100).
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

	jobs    chan job // FIFO queue feeding the workers
	baseCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup // the workers

	mu       sync.Mutex // guards closed, reserved and sends on jobs
	closed   bool
	reserved int // queue slots claimed by Create calls still writing their record
}

// job is one queued review.
type job struct {
	id  string
	ref github.PullRequestRef
	// token is the creating user's own GitHub token, if they were signed in.
	// It lives only in memory for the life of the job and is never stored.
	token string
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

// Create validates the link, records a queued review and starts processing it
// in the background. It returns as soon as the record exists; it never waits
// for GitHub or the analysis.
func (s *Service) Create(ctx context.Context, prURL string) (review.Review, error) {
	ref, err := github.ParsePRURL(prURL)
	if err != nil {
		return review.Review{}, err
	}

	// Reserve a queue slot first, so a refused request never leaves a review
	// behind and the enqueue below can never block.
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

	// Close closes the queue under this lock, so checking closed here means we
	// can never send on a closed channel.
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
	// Room is guaranteed by the reservation. The token comes from the request
	// context, set by the HTTP layer for signed-in users.
	s.jobs <- job{id: rv.ID, ref: ref, token: github.RequestToken(ctx)}
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
	for i := range rv.Findings {
		rv.Findings[i] = s.enrich(rv.Findings[i])
	}
	return rv, nil
}

// List returns recent reviews, newest first. limit is clamped to 1..50, with
// 20 used when it is not positive.
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
	f = s.enrich(f)

	detail := FindingDetail{Finding: f}
	if f.Lesson != "" {
		if sk, ok := s.skills[f.SkillID]; ok {
			if md, err := sk.Lesson(f.Lesson); err != nil {
				slog.Warn("lesson unavailable", "skill", f.SkillID, "lesson", f.Lesson, "error", err)
			} else {
				detail.Lesson = &Lesson{ID: f.Lesson, Markdown: md}
			}
		}
	}
	return detail, nil
}

// enrich fills the display names that come from skill definitions, which the
// database does not store. Unknown skills or checks fall back to their ids.
func (s *Service) enrich(f review.Finding) review.Finding {
	if sk, ok := s.skills[f.SkillID]; ok {
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
	// Events delivers later changes. It is nil when Current is already terminal,
	// and is closed if the subscriber falls behind (reconnect to resume).
	Events <-chan review.Event
	// Close stops the stream. It is always safe to call.
	Close func()
}

// Watch subscribes to a review's status changes. The subscription starts before
// the current status is read, so no change can be missed in between; a change
// may be reported twice, which callers should tolerate.
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

// Close stops accepting reviews and waits for queued and running ones to
// finish. If ctx ends first, the remaining reviews are cancelled and marked
// failed ("interrupted"), and Close returns ctx's error once the workers have
// wound down. Close may be called more than once.
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
