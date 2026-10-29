package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func aiFinding(checkID, file string, confidence float64) ai.Finding {
	return ai.Finding{
		CheckID: checkID, File: file, Line: 5, Evidence: "go func() { <-ch }()",
		Found: "no way to stop", Why: "leaks", Improvement: "add a context", Confidence: confidence,
	}
}

func TestAIPassEnrichesFindingsAndRunsAfterAnalyzing(t *testing.T) {
	store := newFakeStore()
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) {
		return []ai.Finding{aiFinding("goroutine-leak", "main.go", 0.82)}, nil
	}}
	svc := newServiceWithAI(t, store, newFakeSource("main.go"), Config{}, provider)

	rv, _ := svc.Create(context.Background(), prURL)
	stream, err := svc.Watch(context.Background(), rv.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var statuses []review.Status
	statuses = append(statuses, stream.Current.Status)
	for statuses[len(statuses)-1] != review.StatusCompleted {
		e, ok := <-stream.Events
		if !ok {
			t.Fatalf("stream closed early; statuses so far: %v", statuses)
		}
		if e.Status != statuses[len(statuses)-1] {
			statuses = append(statuses, e.Status)
		}
	}
	if !containsStatus(statuses, review.StatusReviewing) {
		t.Fatalf("statuses = %v, want reviewing to appear", statuses)
	}
	// reviewing must come after analyzing and before completed.
	ai_, an := indexOfStatus(statuses, review.StatusReviewing), indexOfStatus(statuses, review.StatusAnalyzing)
	if an == -1 || ai_ < an || ai_ > indexOfStatus(statuses, review.StatusCompleted) {
		t.Fatalf("reviewing out of order: %v", statuses)
	}

	got := waitTerminal(t, store, rv.ID)
	if len(got.Findings) != 1 {
		t.Fatalf("findings = %v", got.Findings)
	}
	f := got.Findings[0]
	if f.Source != review.SourceAI || f.Severity != review.SeverityHigh || f.Confidence != 0.82 ||
		f.SkillName != "Go" || f.CheckTitle != "Goroutine may leak" || f.Lesson != "errors" {
		t.Fatalf("ai finding not enriched from the skill: %+v", f)
	}

	if provider.callCount() != 1 {
		t.Fatalf("provider called %d times, want 1", provider.callCount())
	}
	call := provider.calls[0]
	if call.Skill.ID != "go" || len(call.Checks) != 1 || call.Checks[0].ID != "goroutine-leak" {
		t.Fatalf("provider input = %+v", call)
	}
}

func TestNoAIPassWhenNoProviderConfigured(t *testing.T) {
	store := newFakeStore()
	svc := newService(t, store, newFakeSource("main.go"), Config{})
	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestNoAIPassWhenNoSkillHasAnAICheck(t *testing.T) {
	store := newFakeStore()
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) { return nil, errors.New("should not be called") }}
	// A skill with only a deterministic check: the ai check is stripped out via a bespoke config.
	svc := New(store, store, newFakeSource("main.go"), Config{
		Skills:     onlyDeterministicSkill(t),
		AIProvider: provider,
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = svc.Close(ctx)
	})

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted || provider.callCount() != 0 {
		t.Fatalf("status = %s, provider called %d times; want completed with no AI call", got.Status, provider.callCount())
	}
}

func TestAIProviderErrorDoesNotFailTheReview(t *testing.T) {
	store := newFakeStore()
	analyzer := analyzerFunc(func(context.Context, review.AnalysisInput) ([]review.Finding, error) {
		return []review.Finding{ignoredError("main.go", 1)}, nil
	})
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) { return nil, errors.New("rate limited") }}
	svc := newServiceWithAI(t, store, newFakeSource("main.go"), Config{}, provider, analyzer)

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted {
		t.Fatalf("status = %s (%+v), want completed despite the AI provider failing", got.Status, got.Error)
	}
	if len(got.Findings) != 1 || got.Findings[0].Source != review.SourceDeterministic {
		t.Fatalf("deterministic findings should survive an AI failure: %+v", got.Findings)
	}
}

func TestAIPassOnlyReceivesFilesTheSkillApplies(t *testing.T) {
	store := newFakeStore()
	var sawFiles []string
	provider := &fakeAI{fn: func(input ai.ReviewInput) ([]ai.Finding, error) {
		for _, f := range input.Files {
			sawFiles = append(sawFiles, f.Path)
		}
		return nil, nil
	}}
	svc := newServiceWithAI(t, store, newFakeSource("main.go", "README.md"), Config{}, provider)

	rv, _ := svc.Create(context.Background(), prURL)
	waitTerminal(t, store, rv.ID)
	if len(sawFiles) != 1 || sawFiles[0] != "main.go" {
		t.Fatalf("ai provider saw %v, want only main.go (README.md is not a Go file)", sawFiles)
	}
}

func TestBadAIOutputIsDroppedLikeBadAnalyzerOutput(t *testing.T) {
	store := newFakeStore()
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) {
		unknownCheck := aiFinding("made-up", "main.go", 0.9)
		detCheck := aiFinding("ignored-error", "main.go", 0.9) // claims a deterministic check
		noEvidence := aiFinding("goroutine-leak", "main.go", 0.9)
		noEvidence.Evidence = ""
		good := aiFinding("goroutine-leak", "main.go", 0.9)
		return []ai.Finding{unknownCheck, detCheck, noEvidence, good}, nil
	}}
	svc := newServiceWithAI(t, store, newFakeSource("main.go"), Config{}, provider)

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted || len(got.Findings) != 1 || got.Findings[0].CheckID != "goroutine-leak" {
		t.Fatalf("findings = %+v, want only the one valid ai finding to survive", got.Findings)
	}
}

func TestAIConfidenceIsPreservedNotForcedTo1(t *testing.T) {
	store := newFakeStore()
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) {
		return []ai.Finding{aiFinding("goroutine-leak", "main.go", 0.4)}, nil
	}}
	svc := newServiceWithAI(t, store, newFakeSource("main.go"), Config{}, provider)
	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if len(got.Findings) != 1 || got.Findings[0].Confidence != 0.4 {
		t.Fatalf("findings = %+v, want confidence 0.4 preserved", got.Findings)
	}
}

func TestDeterministicAndAIFindingsAreSortedTogether(t *testing.T) {
	store := newFakeStore()
	analyzer := analyzerFunc(func(context.Context, review.AnalysisInput) ([]review.Finding, error) {
		return []review.Finding{ignoredError("z.go", 1)}, nil // severity high, file z.go
	})
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) {
		return []ai.Finding{aiFinding("goroutine-leak", "a.go", 0.9)}, nil // severity high, file a.go
	}}
	svc := newServiceWithAI(t, store, newFakeSource("main.go"), Config{}, provider, analyzer)

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if len(got.Findings) != 2 {
		t.Fatalf("findings = %+v", got.Findings)
	}
	// Same severity: sorted by file name, so a.go (ai) comes before z.go (deterministic).
	if got.Findings[0].File != "a.go" || got.Findings[1].File != "z.go" {
		t.Fatalf("order = %s, %s; want a.go before z.go", got.Findings[0].File, got.Findings[1].File)
	}
}

func containsStatus(statuses []review.Status, want review.Status) bool {
	return indexOfStatus(statuses, want) != -1
}

func indexOfStatus(statuses []review.Status, want review.Status) int {
	for i, s := range statuses {
		if s == want {
			return i
		}
	}
	return -1
}

func TestBadAIOutputMessageDoesNotLeakIfSkillMissing(t *testing.T) {
	// A finding for a skill not in the review must be dropped, not panic.
	store := newFakeStore()
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) {
		f := aiFinding("goroutine-leak", "main.go", 0.9)
		return []ai.Finding{f}, nil
	}}
	svc := newServiceWithAI(t, store, newFakeSource("main.go"), Config{}, provider)
	rv, err := svc.Create(context.Background(), prURL)
	if err != nil {
		t.Fatal(err)
	}
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted {
		t.Fatalf("status = %s", got.Status)
	}
	if !strings.Contains(got.StatusMessage, "Review complete") {
		t.Errorf("message = %q", got.StatusMessage)
	}
}
