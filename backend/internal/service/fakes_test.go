package service

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

// fakeStore is an in-memory ReviewStore and ReviewRecorder.
type fakeStore struct {
	mu       sync.Mutex
	nextID   int
	reviews  map[string]*review.Review
	rules    map[string][]review.Rule // review id -> rules snapshot
	rulesErr error
}

func newFakeStore() *fakeStore { return &fakeStore{reviews: map[string]*review.Review{}} }

func (f *fakeStore) CreateReview(_ context.Context, owner, repo string, number int) (review.Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := strconv.Itoa(f.nextID)
	rv := &review.Review{
		ID: id, Status: review.StatusQueued, StatusMessage: "Waiting to start", StatusAt: time.Now(), CreatedAt: time.Now(),
		PullRequest: review.PullRequest{Owner: owner, Repo: repo, Number: number},
	}
	f.reviews[id] = rv
	return *rv, nil
}

func (f *fakeStore) GetReview(_ context.Context, id string) (review.Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rv, ok := f.reviews[id]
	if !ok {
		return review.Review{}, review.ErrReviewNotFound
	}
	cp := *rv
	cp.Findings = append([]review.Finding(nil), rv.Findings...)
	cp.Counts = review.CountFindings(cp.Findings)
	return cp, nil
}

func (f *fakeStore) ListReviews(_ context.Context, limit int) ([]review.Review, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []review.Review
	for i := f.nextID; i >= 1 && len(out) < limit; i-- {
		out = append(out, *f.reviews[strconv.Itoa(i)])
	}
	return out, nil
}

func (f *fakeStore) GetFinding(_ context.Context, reviewID, findingID string) (review.Finding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if rv, ok := f.reviews[reviewID]; ok {
		for _, fd := range rv.Findings {
			if fd.ID == findingID {
				return fd, nil
			}
		}
	}
	return review.Finding{}, review.ErrFindingNotFound
}

func (f *fakeStore) ReviewRules(_ context.Context, reviewID string) ([]review.Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rules[reviewID], f.rulesErr
}

func (f *fakeStore) active(id string) (*review.Review, error) {
	rv, ok := f.reviews[id]
	if !ok {
		return nil, review.ErrReviewNotFound
	}
	if rv.Status.Terminal() {
		return nil, review.ErrReviewFinished
	}
	return rv, nil
}

func (f *fakeStore) SetStatus(_ context.Context, id string, status review.Status, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rv, err := f.active(id)
	if err != nil {
		return err
	}
	rv.Status, rv.StatusMessage, rv.StatusAt = status, message, time.Now()
	return nil
}

func (f *fakeStore) SavePullRequest(_ context.Context, id string, pr review.PullRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rv, ok := f.reviews[id]
	if !ok {
		return review.ErrReviewNotFound
	}
	rv.PullRequest = pr
	return nil
}

func (f *fakeStore) CompleteReview(_ context.Context, id string, findings []review.Finding, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rv, err := f.active(id)
	if err != nil {
		return err
	}
	for i := range findings {
		findings[i].ID = strconv.Itoa(i + 1)
	}
	now := time.Now()
	rv.Status, rv.StatusMessage, rv.StatusAt, rv.CompletedAt = review.StatusCompleted, message, now, &now
	rv.Findings = findings
	return nil
}

func (f *fakeStore) FailReview(_ context.Context, id, code, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rv, err := f.active(id)
	if err != nil {
		return err
	}
	now := time.Now()
	rv.Status, rv.StatusMessage, rv.StatusAt, rv.CompletedAt = review.StatusFailed, message, now, &now
	rv.Error = &review.ErrorInfo{Code: code, Message: message}
	return nil
}

// fakeSource is a scriptable PullRequestSource.
type fakeSource struct {
	mu       sync.Mutex
	fetches  int
	pr       func(ctx context.Context) (github.PullRequest, error)
	files    []github.ChangedFile
	filesErr error
	content  func(ctx context.Context, path string) ([]byte, error)

	repoFiles   []github.SourceFile // what RepositoryFiles returns
	repoErr     error
	archiveShas []string // the commits RepositoryFiles was asked for
}

func newFakeSource(files ...string) *fakeSource {
	s := &fakeSource{}
	s.pr = func(context.Context) (github.PullRequest, error) {
		return github.PullRequest{Title: "Add caching", Author: "chirag", URL: "https://github.com/o/r/pull/1", State: "open", HeadSHA: "abc123", FilesChanged: len(files)}, nil
	}
	for _, p := range files {
		s.files = append(s.files, github.ChangedFile{Path: p, Status: "modified", Patch: "@@"})
	}
	s.content = func(_ context.Context, path string) ([]byte, error) { return []byte("package x // " + path), nil }
	return s
}

func (s *fakeSource) FetchPullRequest(ctx context.Context, ref github.PullRequestRef) (github.PullRequest, error) {
	s.mu.Lock()
	s.fetches++
	s.mu.Unlock()
	pr, err := s.pr(ctx)
	pr.Ref = ref
	return pr, err
}

func (s *fakeSource) fetchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fetches
}

func (s *fakeSource) ListFiles(context.Context, github.PullRequestRef) ([]github.ChangedFile, error) {
	return s.files, s.filesErr
}

func (s *fakeSource) FileContent(ctx context.Context, _ github.PullRequestRef, path, _ string) ([]byte, error) {
	return s.content(ctx, path)
}

// RepositoryFiles makes fakeSource a RepositorySource: the whole repository at a
// commit, for the knowledge base.
func (s *fakeSource) RepositoryFiles(_ context.Context, _ github.PullRequestRef, sha string) ([]github.SourceFile, error) {
	s.mu.Lock()
	s.archiveShas = append(s.archiveShas, sha)
	s.mu.Unlock()
	return s.repoFiles, s.repoErr
}

// fakeAI is a scriptable ai.Provider.
type fakeAI struct {
	mu    sync.Mutex
	calls []ai.ReviewInput
	fn    func(ai.ReviewInput) ([]ai.Finding, error)
}

func (f *fakeAI) Review(_ context.Context, input ai.ReviewInput) ([]ai.Finding, error) {
	f.mu.Lock()
	f.calls = append(f.calls, input)
	f.mu.Unlock()
	return f.fn(input)
}

func (f *fakeAI) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// analyzerFunc adapts a function to review.StaticAnalyzer.
type analyzerFunc func(ctx context.Context, in review.AnalysisInput) ([]review.Finding, error)

func (f analyzerFunc) Analyze(ctx context.Context, in review.AnalysisInput) ([]review.Finding, error) {
	return f(ctx, in)
}

// ignoredError is a minimal, valid analyzer finding for the test skill.
func ignoredError(file string, line int) review.Finding {
	return review.Finding{
		SkillID: "go", CheckID: "ignored-error", File: file, Line: line,
		Evidence: "data, _ := os.ReadFile(p)", Found: "error ignored", Why: "silent failure", Improvement: "handle it",
	}
}

const testSkillYAML = `
id: go
name: Go
version: 1
languages: [go]
checks:
  - id: ignored-error
    title: Error return value is ignored
    kind: deterministic
    analyzer: semgrep
    severity: high
    lesson: errors
  - id: goroutine-leak
    title: Goroutine may leak
    kind: ai
    severity: high
    lesson: errors
`

const onlyDeterministicSkillYAML = `
id: go
name: Go
version: 1
languages: [go]
checks:
  - id: ignored-error
    title: Error return value is ignored
    kind: deterministic
    analyzer: semgrep
    severity: high
    lesson: errors
`

// onlyDeterministicSkill is the "go" skill with its ai check removed, for
// tests that need hasAICheck to be false.
func onlyDeterministicSkill(t *testing.T) []skills.Skill {
	t.Helper()
	loaded, err := skills.Load(fstest.MapFS{
		"go/skill.yaml":        {Data: []byte(onlyDeterministicSkillYAML)},
		"go/lessons/errors.md": {Data: []byte("# Errors\n\nHandle them.")},
	})
	if err != nil {
		t.Fatalf("load skill fixture: %v", err)
	}
	return loaded
}

func testSkills(t *testing.T) []skills.Skill {
	t.Helper()
	loaded, err := skills.Load(fstest.MapFS{
		"go/skill.yaml":        {Data: []byte(testSkillYAML)},
		"go/lessons/errors.md": {Data: []byte("# Errors\n\nHandle them.")},
	})
	if err != nil {
		t.Fatalf("load test skills: %v", err)
	}
	return loaded
}

// newService builds a Service over fakes. The store is used for both interfaces.
func newService(t *testing.T, store *fakeStore, src *fakeSource, cfg Config, analyzers ...review.StaticAnalyzer) *Service {
	t.Helper()
	cfg.Skills = testSkills(t)
	cfg.Analyzers = analyzers
	svc := New(store, store, src, cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	return svc
}

// newServiceWithAI is newService plus a scriptable AI provider.
func newServiceWithAI(t *testing.T, store *fakeStore, src *fakeSource, cfg Config, aiProvider ai.Provider, analyzers ...review.StaticAnalyzer) *Service {
	t.Helper()
	cfg.Skills = testSkills(t)
	cfg.Analyzers = analyzers
	cfg.AIProvider = aiProvider
	svc := New(store, store, src, cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})
	return svc
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// waitTerminal waits for a review to finish and returns it.
func waitTerminal(t *testing.T, s *fakeStore, id string) review.Review {
	t.Helper()
	var rv review.Review
	waitFor(t, "review "+id+" to finish", func() bool {
		rv, _ = s.GetReview(context.Background(), id)
		return rv.Status.Terminal()
	})
	return rv
}
