package rules

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

func good() review.Rule {
	return review.Rule{Title: "Wrap errors", Instructions: "Return errors with fmt.Errorf and %w.", Severity: review.SeverityMedium, Enabled: true}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*review.Rule)
		wantErr bool
	}{
		{"valid", func(*review.Rule) {}, false},
		{"empty title", func(r *review.Rule) { r.Title = "   " }, true},
		{"title at the limit", func(r *review.Rule) { r.Title = strings.Repeat("a", MaxTitle) }, false},
		{"title too long", func(r *review.Rule) { r.Title = strings.Repeat("a", MaxTitle+1) }, true},
		{"multi-byte title counts characters not bytes", func(r *review.Rule) { r.Title = strings.Repeat("é", MaxTitle) }, false},
		{"empty instructions", func(r *review.Rule) { r.Instructions = "" }, true},
		{"instructions at the limit", func(r *review.Rule) { r.Instructions = strings.Repeat("a", MaxInstructions) }, false},
		{"instructions too long", func(r *review.Rule) { r.Instructions = strings.Repeat("a", MaxInstructions+1) }, true},
		{"unknown severity", func(r *review.Rule) { r.Severity = "urgent" }, true},
		{"empty severity", func(r *review.Rule) { r.Severity = "" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := good()
			tt.mutate(&r)
			_, err := Validate(r)
			var invalid *InvalidError
			if (err != nil) != tt.wantErr || (err != nil && !errors.As(err, &invalid)) {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTrimsWhitespace(t *testing.T) {
	r := good()
	r.Title, r.Instructions = "  Wrap errors \n", "\t do it  "
	got, err := Validate(r)
	if err != nil || got.Title != "Wrap errors" || got.Instructions != "do it" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

type fakeStore struct{ created, updated int }

func (f *fakeStore) ListRules(context.Context, string) ([]review.Rule, error) { return nil, nil }
func (f *fakeStore) CreateRule(_ context.Context, _ string, r review.Rule, max int) (review.Rule, error) {
	f.created++
	if max != MaxPerUser {
		return r, errors.New("the service must pass its limit to the store")
	}
	return r, nil
}
func (f *fakeStore) UpdateRule(_ context.Context, _, _ string, r review.Rule) (review.Rule, error) {
	f.updated++
	return r, nil
}
func (f *fakeStore) DeleteRule(context.Context, string, string) error { return nil }

func TestInvalidRulesNeverReachTheStore(t *testing.T) {
	st := &fakeStore{}
	svc := NewService(st)
	bad := good()
	bad.Severity = "nope"

	if _, err := svc.Create(context.Background(), "1", bad); err == nil {
		t.Error("Create accepted an invalid rule")
	}
	if _, err := svc.Update(context.Background(), "1", "2", bad); err == nil {
		t.Error("Update accepted an invalid rule")
	}
	if st.created+st.updated != 0 {
		t.Fatal("an invalid rule reached the store")
	}
	if _, err := svc.Create(context.Background(), "1", good()); err != nil {
		t.Fatalf("a valid rule failed: %v", err)
	}
}

func TestSkillTurnsRulesIntoAIChecksWithTheirInstructionsAsTheLesson(t *testing.T) {
	sk, ok := Skill([]review.Rule{
		{ID: "7", Title: "Wrap errors", Instructions: "Use %w.", Severity: review.SeverityHigh, Enabled: true},
		{ID: "9", Title: "No globals", Instructions: "Avoid package-level state.", Severity: review.SeveritySuggestion, Enabled: true},
	})
	if !ok || sk.ID != SkillID || sk.Name != SkillName {
		t.Fatalf("skill = %+v, ok %v", sk, ok)
	}
	if !sk.AppliesTo("python") || !sk.AppliesTo("go") {
		t.Error("custom rules should apply to every language")
	}

	check, found := sk.Check("rule-7")
	if !found || check.Kind != skills.KindAI || check.Severity != review.SeverityHigh || check.Title != "Wrap errors" || check.Lesson != "rule-7" {
		t.Fatalf("check = %+v, found %v", check, found)
	}
	lesson, err := sk.Lesson("rule-9")
	if err != nil || !strings.Contains(lesson, "Avoid package-level state.") {
		t.Fatalf("lesson = %q, %v", lesson, err)
	}
	if _, err := sk.Lesson("rule-404"); err == nil {
		t.Error("an unknown lesson should be an error, not an empty string")
	}
}

func TestSkillOfNoRulesIsNotASkill(t *testing.T) {
	if _, ok := Skill(nil); ok {
		t.Fatal("no rules should give no skill")
	}
}

func TestPlaceholderMatchesTheSkillIDFindingsUse(t *testing.T) {
	if p := Placeholder(); p.ID != SkillID || p.Name != SkillName || len(p.Languages) == 0 {
		t.Fatalf("placeholder = %+v", p)
	}
}
