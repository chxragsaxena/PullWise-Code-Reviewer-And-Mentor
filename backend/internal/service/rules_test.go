package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/rules"
)

var userRules = []review.Rule{
	{ID: "7", Title: "Wrap errors", Instructions: "Return errors with fmt.Errorf and %w.", Severity: review.SeverityHigh, Enabled: true},
	{ID: "9", Title: "No globals", Instructions: "Avoid package-level state.", Severity: review.SeveritySuggestion, Enabled: true},
}

// reviewWithRules runs a review whose id-1 snapshot holds userRules.
func reviewWithRules(t *testing.T, provider *fakeAI) (*Service, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	store.rules = map[string][]review.Rule{"1": userRules}
	svc := newServiceWithAI(t, store, newFakeSource("main.go"), Config{}, provider)
	return svc, store
}

func TestCustomRulesGoToTheAIAsOneSkillWithTheirInstructions(t *testing.T) {
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) { return nil, nil }}
	svc, store := reviewWithRules(t, provider)

	rv, _ := svc.Create(context.Background(), prURL)
	waitTerminal(t, store, rv.ID)

	var custom *ai.ReviewInput
	for i := range provider.calls {
		if provider.calls[i].Skill.ID == rules.SkillID {
			custom = &provider.calls[i]
		}
	}
	if custom == nil {
		t.Fatal("the AI was never asked to apply the user's rules")
	}
	if len(custom.Checks) != 2 || custom.Checks[0].ID != "rule-7" || custom.Checks[1].ID != "rule-9" {
		t.Fatalf("checks = %+v, want one per rule", custom.Checks)
	}
	if len(custom.Files) != 1 || custom.Files[0].Path != "main.go" {
		t.Fatalf("files = %+v", custom.Files)
	}
	for check, want := range map[string]string{"rule-7": "Return errors with fmt.Errorf and %w.", "rule-9": "Avoid package-level state."} {
		if lesson, err := custom.Skill.Lesson(check); err != nil || !strings.Contains(lesson, want) {
			t.Errorf("the lesson for %s = %q, %v; want the user's instructions", check, lesson, err)
		}
	}
}

func TestFindingsFromCustomRulesTakeTheRulesSeverityAndAreLabelled(t *testing.T) {
	provider := &fakeAI{fn: func(in ai.ReviewInput) ([]ai.Finding, error) {
		if in.Skill.ID != rules.SkillID {
			return nil, nil
		}
		return []ai.Finding{
			aiFinding("rule-7", "main.go", 0.7),
			aiFinding("rule-404", "main.go", 0.9), // a rule the user never wrote
			aiFinding("ignored-error", "main.go", 0.9),
		}, nil
	}}
	svc, store := reviewWithRules(t, provider)
	rv, _ := svc.Create(context.Background(), prURL)
	waitTerminal(t, store, rv.ID)

	got, err := svc.Get(context.Background(), rv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("got %d findings, want only the one for a real rule: %+v", len(got.Findings), got.Findings)
	}
	f := got.Findings[0]
	if f.SkillID != rules.SkillID || f.CheckID != "rule-7" || f.Severity != review.SeverityHigh || f.Source != review.SourceAI {
		t.Fatalf("finding = %+v, want the rule's severity (high), not the model's", f)
	}
	if f.SkillName != "Your standards" || f.CheckTitle != "Wrap errors" {
		t.Fatalf("labels = %q / %q, want the skill name and the rule's title", f.SkillName, f.CheckTitle)
	}
	if f.Confidence != 0.7 {
		t.Errorf("confidence = %v, want the model's own 0.7", f.Confidence)
	}
}

func TestACustomFindingsLessonIsTheUsersInstructions(t *testing.T) {
	provider := &fakeAI{fn: func(in ai.ReviewInput) ([]ai.Finding, error) {
		if in.Skill.ID != rules.SkillID {
			return nil, nil
		}
		return []ai.Finding{aiFinding("rule-9", "main.go", 0.8)}, nil
	}}
	svc, store := reviewWithRules(t, provider)
	rv, _ := svc.Create(context.Background(), prURL)
	done := waitTerminal(t, store, rv.ID)

	detail, err := svc.GetFinding(context.Background(), rv.ID, done.Findings[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Finding.CheckTitle != "No globals" || detail.Lesson == nil || !strings.Contains(detail.Lesson.Markdown, "Avoid package-level state.") {
		t.Fatalf("detail = %+v, lesson = %+v", detail.Finding, detail.Lesson)
	}
}

func TestTheUsersOwnRulesAreAskedFirstThenSkillsByID(t *testing.T) {
	var order []string
	provider := &fakeAI{fn: func(in ai.ReviewInput) ([]ai.Finding, error) {
		order = append(order, in.Skill.ID)
		return nil, nil
	}}
	svc, store := reviewWithRules(t, provider)
	rv, _ := svc.Create(context.Background(), prURL)
	waitTerminal(t, store, rv.ID)

	if len(order) < 2 || order[0] != rules.SkillID {
		t.Fatalf("AI call order = %v, want the user's own rules first", order)
	}
}

func TestReviewsWithoutRulesAreUnchanged(t *testing.T) {
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) { return nil, nil }}
	svc := newServiceWithAI(t, newFakeStore(), newFakeSource("main.go"), Config{}, provider)
	rv, _ := svc.Create(context.Background(), prURL)
	waitTerminal(t, svc.store.(*fakeStore), rv.ID)

	for _, call := range provider.calls {
		if call.Skill.ID == rules.SkillID {
			t.Fatal("the custom skill ran although the user has no rules")
		}
	}
}

func TestTroubleReadingRulesNeverFailsTheReview(t *testing.T) {
	provider := &fakeAI{fn: func(ai.ReviewInput) ([]ai.Finding, error) { return nil, nil }}
	store := newFakeStore()
	store.rulesErr = errors.New("database hiccup")
	svc := newServiceWithAI(t, store, newFakeSource("main.go"), Config{}, provider)

	rv, _ := svc.Create(context.Background(), prURL)
	got := waitTerminal(t, store, rv.ID)
	if got.Status != review.StatusCompleted {
		t.Fatalf("status = %s, want completed", got.Status)
	}
}

func TestRulesAreNotReadWhenThereIsNoAIPass(t *testing.T) {
	store := newFakeStore()
	store.rulesErr = errors.New("must not be called")
	store.rules = map[string][]review.Rule{"1": userRules}
	svc := newService(t, store, newFakeSource("main.go"), Config{})

	rv, _ := svc.Create(context.Background(), prURL)
	if got := waitTerminal(t, store, rv.ID); got.Status != review.StatusCompleted {
		t.Fatalf("status = %s", got.Status)
	}
}
