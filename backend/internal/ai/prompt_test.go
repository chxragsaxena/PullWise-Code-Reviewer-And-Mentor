package ai

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

func testSkill(t *testing.T) skills.Skill {
	t.Helper()
	loaded, err := skills.Load(fstest.MapFS{
		"go/skill.yaml": {Data: []byte(`
id: go
name: Go
version: 1
languages: [go]
description: Idiomatic Go.
checks:
  - id: goroutine-leak-risk
    title: Goroutine started with no way to stop or wait for it
    kind: ai
    severity: high
    lesson: concurrency
`)},
		"go/lessons/concurrency.md": {Data: []byte("# Concurrency\n\nA goroutine needs a way to stop.")},
	})
	if err != nil {
		t.Fatalf("load test skill: %v", err)
	}
	return loaded[0]
}

func TestBuildPromptIncludesSkillCheckAndLesson(t *testing.T) {
	sk := testSkill(t)
	prompt := buildPrompt(ReviewInput{
		Skill:  sk,
		Checks: sk.Checks,
		Files:  []review.ChangedFile{{Path: "worker.go", Patch: "@@ -1 +1 @@\n+go func() { <-ch }()"}},
	})

	for _, want := range []string{
		"Go", "Idiomatic Go.",
		"Goroutine started with no way to stop or wait for it", "goroutine-leak-risk",
		"A goroutine needs a way to stop.",
		"worker.go", "go func() { <-ch }()",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, prompt)
		}
	}
}

func TestBuildPromptSkipsFilesWithNoPatch(t *testing.T) {
	sk := testSkill(t)
	prompt := buildPrompt(ReviewInput{Skill: sk, Checks: sk.Checks, Files: []review.ChangedFile{{Path: "binary.png", Patch: ""}}})
	if strings.Contains(prompt, "binary.png") {
		t.Fatalf("a file with no patch should not appear:\n%s", prompt)
	}
}

func TestWriteFilesRespectsTheBudget(t *testing.T) {
	var b strings.Builder
	big := strings.Repeat("x", maxPromptChars-40)
	files := []review.ChangedFile{
		{Path: "a.go", Patch: big},
		{Path: "b.go", Patch: "small"},
		{Path: "c.go", Patch: "small"},
	}
	writeFiles(&b, files)
	out := b.String()

	if !strings.Contains(out, "a.go") {
		t.Fatal("the first file should always be included")
	}
	if strings.Contains(out, "b.go") || strings.Contains(out, "c.go") {
		t.Fatalf("files past the budget should be omitted:\n%s", out)
	}
	if !strings.Contains(out, "2 more changed file(s) omitted") {
		t.Fatalf("expected an omission note counting the remaining files, got:\n%s", out)
	}
}

func TestWriteFilesUnderBudgetIncludesEverything(t *testing.T) {
	var b strings.Builder
	files := []review.ChangedFile{{Path: "a.go", Patch: "x"}, {Path: "b.go", Patch: "y"}}
	writeFiles(&b, files)
	out := b.String()
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") || strings.Contains(out, "omitted") {
		t.Fatalf("both small files should fit with no omission note:\n%s", out)
	}
}
