package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

const prURL = "https://github.com/o/r/pull/1"

func TestCreateReturnsQuicklyWithoutWaitingForGitHub(t *testing.T) {
	store := newFakeStore()
	gate := make(chan struct{})
	src := newFakeSource("main.go")
	src.pr = func(ctx context.Context) (github.PullRequest, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return github.PullRequest{HeadSHA: "abc"}, nil
	}
	svc := newService(t, store, src, Config{})

	start := time.Now()
	rv, err := svc.Create(context.Background(), prURL)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Create waited for GitHub")
	}
	if rv.Status != review.StatusQueued || rv.PullRequest.Owner != "o" || rv.PullRequest.Number != 1 {
		t.Fatalf("unexpected review: %+v", rv)
	}
	close(gate)
	waitTerminal(t, store, rv.ID)
}

func TestHappyPathProducesEnrichedFindingsAndProgressEvents(t *testing.T) {
	store := newFakeStore()
	gate := make(chan struct{})
	src := newFakeSource("cmd/main.go", "README.md")
	src.pr = func(context.Context) (github.PullRequest, error) {
		<-gate // hold the pipeline so the subscriber sees every step
		return github.PullRequest{Title: "Add caching", Author: "chirag", HeadSHA: "abc123"}, nil
	}
	var analyzed []string
	analyzer := analyzerFunc(func(_ context.Context, in review.AnalysisInput) ([]review.Finding, error) {
		for _, f := range in.Files {
			analyzed = append(analyzed, f.Path)
		}
		return []review.Finding{ignoredError("cmd/main.go", 12)}, nil
	})
	svc := newService(t, store, src, Config{}, analyzer)

	rv, err := svc.Create(context.Background(), prURL)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := svc.Watch(context.Background(), rv.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	close(gate)

	seen := []review.Status{stream.Current.Status}
	timeout := time.After(3 * time.Second)
	for !seen[len(seen)-1].Terminal() {
		select {
		case e := <-stream.Events:
			if e.Status != seen[len(seen)-1] {
				seen = append(seen, e.Status)
			}
		case <-timeout:
			t.Fatalf("timed out; statuses so far: %v", seen)
		}
	}
	// The stream may join partway through, but must end in order at completed.
	order := map[review.Status]int{review.StatusQueued: 0, review.StatusFetchingPR: 1, review.StatusAnalyzing: 2, review.StatusCompleted: 3}
	for i := 1; i < len(seen); i++ {
		if order[seen[i]] <= order[seen[i-1]] {
			t.Fatalf("statuses out of order: %v", seen)
		}
	}
	if seen[len(seen)-1] != review.StatusCompleted {
		t.Fatalf("final status = %v, want completed", seen[len(seen)-1])
	}

	got, err := svc.Get(context.Background(), rv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PullRequest.Title != "Add caching" || got.PullRequest.HeadSHA != "abc123" {
		t.Errorf("pull request not saved: %+v", got.PullRequest)
	}
	if len(analyzed) != 1 || analyzed[0] != "cmd/main.go" {
		t.Errorf("analyzer saw %v; only the Go file should be analyzed", analyzed)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(got.Findings))
	}
	f := got.Findings[0]
	if f.Severity != review.SeverityHigh || f.Source != review.SourceDeterministic || f.Confidence != 1 ||
		f.SkillName != "Go" || f.CheckTitle != "Error return value is ignored" || f.Lesson != "errors" {
		t.Errorf("finding not filled from the skill definition: %+v", f)
	}
	if got.Counts.High != 1 {
		t.Errorf("counts = %+v", got.Counts)
	}
}

func TestFailureMapping(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*fakeSource)
		analyzer  review.StaticAnalyzer
		wantCode  string
	}{
		{"pull request not found", func(s *fakeSource) {
			s.pr = func(context.Context) (github.PullRequest, error) {
				return github.PullRequest{}, &github.APIError{Status: 404}
			}
		}, nil, "pr_not_found"},
		{"github rate limit", func(s *fakeSource) {
			s.pr = func(context.Context) (github.PullRequest, error) {
				return github.PullRequest{}, &github.APIError{Status: 403, RateLimited: true}
			}
		}, nil, "github_rate_limited"},
		{"listing files fails", func(s *fakeSource) { s.filesErr = errors.New("boom") }, nil, "internal_error"},
		{"downloading a file fails", func(s *fakeSource) {
			s.content = func(context.Context, string) ([]byte, error) { return nil, errors.New("connection reset") }
		}, nil, "internal_error"},
		{"analyzer fails", func(*fakeSource) {}, analyzerFunc(func(context.Context, review.AnalysisInput) ([]review.Finding, error) {
			return nil, errors.New("semgrep exploded")
		}), "analysis_failed"},
		{"analyzer panics", func(*fakeSource) {}, analyzerFunc(func(context.Context, review.AnalysisInput) ([]review.Finding, error) {
			panic("nil map")
		}), "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			src := newFakeSource("main.go")
			tt.configure(src)
			var analyzers []review.StaticAnalyzer
			if tt.analyzer != nil {
				analyzers = append(analyzers, tt.analyzer)
			}
			svc := newService(t, store, src, Config{}, analyzers...)

			rv, err := svc.Create(context.Background(), prURL)
			if err != nil {
				t.Fatal(err)
			}
			got := waitTerminal(t, store, rv.ID)
			if got.Status != review.StatusFailed || got.Error == nil || got.Error.Code != tt.wantCode {
				t.Fatalf("got status %s, error %+v; want failed with code %q", got.Status, got.Error, tt.wantCode)
			}
			if strings.Contains(got.Error.Message, "semgrep exploded") || strings.Contains(got.Error.Message, "connection reset") {
				t.Errorf("internal detail leaked into the user-facing message: %q", got.Error.Message)
			}
		})
	}
}

func TestServiceSurvivesAPanickingAnalyzer(t *testing.T) {
	store := newFakeStore()
	calls := 0
	analyzer := analyzerFunc(func(context.Context, review.AnalysisInput) ([]review.Finding, error) {
		calls++
		if calls == 1 {
			panic("first call blows up")
		}
		return nil, nil
	})
	svc := newService(t, store, newFakeSource("main.go"), Config{MaxConcurrent: 1}, analyzer)

	first, _ := svc.Create(context.Background(), prURL)
	if got := waitTerminal(t, store, first.ID); got.Status != review.StatusFailed {
		t.Fatalf("first review status = %s, want failed", got.Status)
	}
	second, _ := svc.Create(context.Background(), prURL)
	if got := waitTerminal(t, store, second.ID); got.Status != review.StatusCompleted {
		t.Fatalf("second review status = %s, want completed (slot must be released after a panic)", got.Status)
	}
}

func TestReviewTimesOut(t *testing.T) {
	store := newFakeStore()
	src := newFakeSource("main.go")
	src.pr = func(ctx context.Context) (github.PullRequest, error) {
		<-ctx.Done() // a GitHub call that never answers
		return github.PullRequest{}, ctx.Err()
	}
	svc := newService(t, store, src, Config{ReviewTimeout: 50 * time.Millisecond})

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Error == nil || got.Error.Code != "timeout" {
		t.Fatalf("error = %+v, want timeout", got.Error)
	}
}

func TestOnlySupportedLanguagesTriggerAnalysis(t *testing.T) {
	store := newFakeStore()
	called := false
	analyzer := analyzerFunc(func(context.Context, review.AnalysisInput) ([]review.Finding, error) {
		called = true
		return nil, nil
	})
	svc := newService(t, store, newFakeSource("README.md", "docs/guide.md", "script.py"), Config{}, analyzer)

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted || len(got.Findings) != 0 {
		t.Fatalf("got %s with %d findings, want completed with none", got.Status, len(got.Findings))
	}
	if called {
		t.Error("analyzer ran although no file is in a supported language")
	}
	if !strings.Contains(got.StatusMessage, "nothing to check") {
		t.Errorf("message = %q, want it to explain nothing was checked", got.StatusMessage)
	}
}

func TestRemovedFilesAreNotAnalyzed(t *testing.T) {
	store := newFakeStore()
	src := newFakeSource("gone.go", "kept.go")
	src.files[0].Status = "removed"
	var saw []string
	analyzer := analyzerFunc(func(_ context.Context, in review.AnalysisInput) ([]review.Finding, error) {
		for _, f := range in.Files {
			saw = append(saw, f.Path)
		}
		return nil, nil
	})
	svc := newService(t, store, src, Config{}, analyzer)

	rv, _ := svc.Create(context.Background(), prURL)
	waitTerminal(t, store, rv.ID)
	if len(saw) != 1 || saw[0] != "kept.go" {
		t.Fatalf("analyzed %v, want only kept.go", saw)
	}
}

func TestFileCapAndOversizedFiles(t *testing.T) {
	store := newFakeStore()
	src := newFakeSource("a.go", "b.go", "c.go", "d.go", "e.go")
	src.content = func(_ context.Context, path string) ([]byte, error) {
		if path == "a.go" {
			return nil, github.ErrTooLarge // skipped, not fatal
		}
		return []byte("package x"), nil
	}
	var saw []string
	analyzer := analyzerFunc(func(_ context.Context, in review.AnalysisInput) ([]review.Finding, error) {
		for _, f := range in.Files {
			saw = append(saw, f.Path)
		}
		return nil, nil
	})
	svc := newService(t, store, src, Config{MaxFilesToAnalyze: 3}, analyzer)

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted {
		t.Fatalf("status = %s (%+v)", got.Status, got.Error)
	}
	if strings.Join(saw, ",") != "b.go,c.go" { // first 3 = a,b,c; a is oversized
		t.Fatalf("analyzed %v, want b.go,c.go", saw)
	}
	if !strings.Contains(got.StatusMessage, "Analyzed 3 of 5") {
		t.Errorf("message = %q, want it to say the cap applied", got.StatusMessage)
	}
}

func TestBadAnalyzerOutputIsDroppedAndTheRestIsOrdered(t *testing.T) {
	store := newFakeStore()
	analyzer := analyzerFunc(func(context.Context, review.AnalysisInput) ([]review.Finding, error) {
		unknownCheck := ignoredError("x.go", 1)
		unknownCheck.CheckID = "made-up"
		aiCheck := ignoredError("x.go", 2)
		aiCheck.CheckID = "goroutine-leak" // an AI check: analyzers may not report it
		otherSkill := ignoredError("x.go", 3)
		otherSkill.SkillID = "python"
		noEvidence := ignoredError("x.go", 4)
		noEvidence.Evidence = ""
		selfDeclared := ignoredError("b.go", 9) // tries to claim critical/AI: must be overridden
		selfDeclared.Severity, selfDeclared.Source, selfDeclared.Confidence = review.SeverityCritical, review.SourceAI, 0.3
		return []review.Finding{unknownCheck, aiCheck, otherSkill, noEvidence, ignoredError("b.go", 20), selfDeclared, ignoredError("a.go", 5)}, nil
	})
	svc := newService(t, store, newFakeSource("a.go", "b.go", "x.go"), Config{}, analyzer)

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted {
		t.Fatalf("status = %s", got.Status)
	}
	var locs []string
	for _, f := range got.Findings {
		locs = append(locs, f.File+":"+itoa(f.Line))
		if f.Severity != review.SeverityHigh || f.Source != review.SourceDeterministic || f.Confidence != 1 {
			t.Errorf("analyzer overrode the skill definition: %+v", f)
		}
	}
	if strings.Join(locs, ",") != "a.go:5,b.go:9,b.go:20" {
		t.Fatalf("findings = %v, want the 3 valid ones sorted by file then line", locs)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestConcurrencyLimitKeepsExtraReviewsQueued(t *testing.T) {
	store := newFakeStore()
	gate := make(chan struct{})
	src := newFakeSource("main.go")
	src.pr = func(ctx context.Context) (github.PullRequest, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return github.PullRequest{HeadSHA: "abc"}, nil
	}
	svc := newService(t, store, src, Config{MaxConcurrent: 1})

	first, _ := svc.Create(context.Background(), prURL)
	second, _ := svc.Create(context.Background(), prURL)

	waitFor(t, "first review to reach GitHub", func() bool { return src.fetchCount() == 1 })
	time.Sleep(50 * time.Millisecond) // give the second a chance to (wrongly) start
	if src.fetchCount() != 1 {
		t.Fatal("second review started although MaxConcurrent is 1")
	}
	if got, _ := store.GetReview(context.Background(), second.ID); got.Status != review.StatusQueued {
		t.Fatalf("second review status = %s, want queued", got.Status)
	}

	close(gate)
	waitTerminal(t, store, first.ID)
	if got := waitTerminal(t, store, second.ID); got.Status != review.StatusCompleted {
		t.Fatalf("second review ended %s", got.Status)
	}
}

func TestBusyWhenTooManyReviewsArePending(t *testing.T) {
	store := newFakeStore()
	gate := make(chan struct{})
	src := newFakeSource("main.go")
	src.pr = func(ctx context.Context) (github.PullRequest, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return github.PullRequest{}, nil
	}
	// One worker and room for one waiting review.
	svc := newService(t, store, src, Config{MaxConcurrent: 1, MaxPending: 1})

	running, err := svc.Create(context.Background(), prURL)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the worker to pick up the first review", func() bool { return src.fetchCount() == 1 })

	waiting, err := svc.Create(context.Background(), prURL) // fills the queue
	if err != nil {
		t.Fatalf("second Create should be queued: %v", err)
	}
	if _, err := svc.Create(context.Background(), prURL); !errors.Is(err, ErrBusy) {
		t.Fatalf("third Create error = %v, want ErrBusy", err)
	}
	if n := len(store.reviews); n != 2 {
		t.Fatalf("store has %d reviews; a refused request must not create one", n)
	}

	close(gate)
	waitTerminal(t, store, running.ID)
	waitTerminal(t, store, waiting.ID)
	waitFor(t, "queue space to free up", func() bool {
		_, err := svc.Create(context.Background(), prURL)
		return err == nil
	})
}

func TestQueuedReviewsRunInTheOrderTheyWereCreated(t *testing.T) {
	store := newFakeStore()
	svc := newService(t, store, newFakeSource("main.go"), Config{MaxConcurrent: 1, MaxPending: 20})

	var ids []string
	for range 10 {
		rv, err := svc.Create(context.Background(), prURL)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rv.ID)
	}
	for _, id := range ids {
		waitTerminal(t, store, id)
	}

	// With a single worker, reviews must finish in the order they were created.
	for i := 1; i < len(ids); i++ {
		earlier, _ := store.GetReview(context.Background(), ids[i-1])
		later, _ := store.GetReview(context.Background(), ids[i])
		if later.CompletedAt.Before(*earlier.CompletedAt) {
			t.Fatalf("review %s finished before review %s, which was created first", ids[i], ids[i-1])
		}
	}
}

func TestCreateRejectsBadLinksWithoutCreatingAnything(t *testing.T) {
	store := newFakeStore()
	svc := newService(t, store, newFakeSource(), Config{})

	for _, bad := range []string{"", "not a url", "https://gitlab.com/o/r/pull/1", "https://github.com/o/r/issues/1"} {
		if _, err := svc.Create(context.Background(), bad); !errors.Is(err, ErrInvalidPRURL) {
			t.Errorf("Create(%q) error = %v, want ErrInvalidPRURL", bad, err)
		}
	}
	if len(store.reviews) != 0 {
		t.Fatalf("%d reviews created for invalid links", len(store.reviews))
	}
}

func TestCloseWaitsForRunningReviewsThenRefusesNewOnes(t *testing.T) {
	store := newFakeStore()
	gate := make(chan struct{})
	src := newFakeSource("main.go")
	src.pr = func(ctx context.Context) (github.PullRequest, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return github.PullRequest{}, nil
	}
	svc := newService(t, store, src, Config{})
	rv, _ := svc.Create(context.Background(), prURL)
	waitFor(t, "review to start", func() bool { return src.fetchCount() == 1 })

	closed := make(chan error, 1)
	go func() { closed <- svc.Close(context.Background()) }()

	select {
	case <-closed:
		t.Fatal("Close returned while a review was still running")
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := svc.Create(context.Background(), prURL); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("Create during shutdown = %v, want ErrShuttingDown", err)
	}

	close(gate)
	if err := <-closed; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got, _ := store.GetReview(context.Background(), rv.ID); got.Status != review.StatusCompleted {
		t.Fatalf("running review ended %s, want it allowed to finish", got.Status)
	}
}

func TestCloseDeadlineInterruptsReviewsAndRecordsIt(t *testing.T) {
	store := newFakeStore()
	src := newFakeSource("main.go")
	src.pr = func(ctx context.Context) (github.PullRequest, error) {
		<-ctx.Done() // never finishes on its own
		return github.PullRequest{}, ctx.Err()
	}
	svc := newService(t, store, src, Config{})
	rv, _ := svc.Create(context.Background(), prURL)
	waitFor(t, "review to start", func() bool { return src.fetchCount() == 1 })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := svc.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close = %v, want DeadlineExceeded", err)
	}
	got, _ := store.GetReview(context.Background(), rv.ID)
	if got.Status != review.StatusFailed || got.Error == nil || got.Error.Code != "interrupted" {
		t.Fatalf("review = %s / %+v, want failed with code interrupted", got.Status, got.Error)
	}
}

func TestWatch(t *testing.T) {
	store := newFakeStore()
	svc := newService(t, store, newFakeSource("main.go"), Config{})

	if _, err := svc.Watch(context.Background(), "999"); !errors.Is(err, review.ErrReviewNotFound) {
		t.Fatalf("Watch unknown = %v, want ErrReviewNotFound", err)
	}

	rv, _ := svc.Create(context.Background(), prURL)
	waitTerminal(t, store, rv.ID)
	stream, err := svc.Watch(context.Background(), rv.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if stream.Current.Status != review.StatusCompleted || stream.Events != nil {
		t.Fatalf("a finished review should report its terminal status and no live stream: %+v", stream)
	}
}

func TestGetFindingIncludesLessonText(t *testing.T) {
	store := newFakeStore()
	analyzer := analyzerFunc(func(context.Context, review.AnalysisInput) ([]review.Finding, error) {
		return []review.Finding{ignoredError("main.go", 3)}, nil
	})
	svc := newService(t, store, newFakeSource("main.go"), Config{}, analyzer)

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)

	detail, err := svc.GetFinding(context.Background(), rv.ID, got.Findings[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Lesson == nil || detail.Lesson.ID != "errors" || !strings.Contains(detail.Lesson.Markdown, "Handle them.") {
		t.Fatalf("lesson = %+v", detail.Lesson)
	}
	if detail.Finding.SkillName != "Go" {
		t.Errorf("finding not enriched: %+v", detail.Finding)
	}
	if _, err := svc.GetFinding(context.Background(), rv.ID, "nope"); !errors.Is(err, review.ErrFindingNotFound) {
		t.Errorf("unknown finding error = %v", err)
	}
}

func TestListClampsTheLimit(t *testing.T) {
	store := newFakeStore()
	svc := newService(t, store, newFakeSource("README.md"), Config{MaxPending: 1000})
	for range 60 {
		if _, err := svc.Create(context.Background(), prURL); err != nil {
			t.Fatal(err)
		}
	}
	for limit, want := range map[int]int{0: 20, -5: 20, 3: 3, 50: 50, 500: 50} {
		got, err := svc.List(context.Background(), limit)
		if err != nil || len(got) != want {
			t.Errorf("List(%d) = %d reviews, %v; want %d", limit, len(got), err, want)
		}
	}
}
