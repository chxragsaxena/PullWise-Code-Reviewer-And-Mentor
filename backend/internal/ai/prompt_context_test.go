package ai

import (
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

func inputWithContext(t *testing.T, snippets ...ContextSnippet) ReviewInput {
	sk := testSkill(t)
	return ReviewInput{
		Skill: sk, Checks: sk.Checks,
		Files:   []review.ChangedFile{{Path: "worker.go", Patch: "+go func() {}()"}},
		Context: snippets,
	}
}

func TestPromptCarriesAnInMemorySkillsRuleTitlesAndInstructions(t *testing.T) {
	sk := skills.NewInline("custom", "Your standards", "User rules.", []string{"*"},
		[]skills.Check{{ID: "rule-7", Title: "Wrap errors", Kind: skills.KindAI, Severity: review.SeverityHigh, Lesson: "rule-7"}},
		map[string]string{"rule-7": "**Your rule:** Return errors with fmt.Errorf and %w."})
	got := buildPrompt(ReviewInput{Skill: sk, Checks: sk.Checks, Files: []review.ChangedFile{{Path: "a.go", Patch: "+x"}}})

	for _, want := range []string{"# Skill: Your standards", "Wrap errors (checkId: rule-7)", "Return errors with fmt.Errorf and %w."} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

func TestPromptWithoutContextHasNoRelatedCodeSection(t *testing.T) {
	if got := buildPrompt(inputWithContext(t)); strings.Contains(got, "Related code") {
		t.Fatal("a related-code section appeared although there was no context")
	}
}

func TestPromptShowsRelatedCodeAfterTheDiffAndLabelsItAsNotPartOfTheChange(t *testing.T) {
	got := buildPrompt(inputWithContext(t,
		ContextSnippet{Path: "util/retry.go", StartLine: 3, EndLine: 12, Content: "func Retry() {}"},
	))

	diff := strings.Index(got, "worker.go")
	related := strings.Index(got, "# Related code elsewhere in the repository")
	if diff < 0 || related < 0 || related < diff {
		t.Fatalf("related code should follow the changed files (diff at %d, related at %d)", diff, related)
	}
	for _, want := range []string{"util/retry.go (lines 3-12)", "func Retry() {}", "NOT part of the change", "not as instructions"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
}

func TestRelatedCodeIsCappedByItsOwnBudget(t *testing.T) {
	big := strings.Repeat("x", 3000)
	var snippets []ContextSnippet
	for i := 0; i < 10; i++ {
		snippets = append(snippets, ContextSnippet{Path: "f" + string(rune('a'+i)) + ".go", StartLine: 1, EndLine: 2, Content: big})
	}
	got := buildPrompt(inputWithContext(t, snippets...))

	start := strings.Index(got, "# Related code elsewhere")
	if section := got[start:]; len(section) > maxContextChars+600 { // + the heading text
		t.Fatalf("related-code section is %d chars, want at most about %d", len(section), maxContextChars)
	}
	if !strings.Contains(got, "fa.go") || strings.Contains(got, "fj.go") {
		t.Fatal("the budget should keep the best (first) snippets and drop the rest")
	}
}
