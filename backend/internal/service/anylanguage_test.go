package service

import (
	"context"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

// designSkillYAML is an AI-only skill that applies to every language.
const designSkillYAML = `
id: design
name: Design
version: 1
languages: ["*"]
checks:
  - id: god-function
    title: Function does too much
    kind: ai
    severity: medium
    lesson: design
`

func skillsWithAnyLanguage(t *testing.T) []skills.Skill {
	t.Helper()
	loaded, err := skills.Load(fstest.MapFS{
		"go/skill.yaml":            {Data: []byte(testSkillYAML)},
		"go/lessons/errors.md":     {Data: []byte("# Errors")},
		"design/skill.yaml":        {Data: []byte(designSkillYAML)},
		"design/lessons/design.md": {Data: []byte("# Design")},
	})
	if err != nil {
		t.Fatalf("load skills: %v", err)
	}
	return loaded
}

// A PR in a language with no analyzer still gets any-language AI skills, not the Go skill.
func TestAnyLanguageSkillsReviewLanguagesWithoutAnalyzers(t *testing.T) {
	tests := []struct {
		name       string
		files      []string
		wantSkills []string // skills the AI was asked to run, sorted
		wantFiles  []string // files the design skill saw, sorted
	}{
		{"python only", []string{"app.py", "notes.md", "data.json"}, []string{"design"}, []string{"app.py"}},
		{"python and go: each skill gets its own files", []string{"app.py", "main.go"}, []string{"design", "go"}, []string{"app.py", "main.go"}},
		{"nothing reviewable at all", []string{"README.md", "data.json"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			sawSkill := map[string][]string{}
			provider := &fakeAI{fn: func(in ai.ReviewInput) ([]ai.Finding, error) {
				mu.Lock()
				defer mu.Unlock()
				for _, f := range in.Files {
					sawSkill[in.Skill.ID] = append(sawSkill[in.Skill.ID], f.Path)
				}
				return nil, nil
			}}

			store := newFakeStore()
			svc := New(store, store, newFakeSource(tt.files...), Config{
				Skills: skillsWithAnyLanguage(t), AIProvider: provider,
			})
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = svc.Close(ctx)
			})

			rv, err := svc.Create(context.Background(), prURL)
			if err != nil {
				t.Fatal(err)
			}
			got := waitTerminal(t, store, rv.ID)
			if got.Status != review.StatusCompleted {
				t.Fatalf("status = %s (%v), want completed", got.Status, got.Error)
			}

			mu.Lock()
			defer mu.Unlock()
			var ran []string
			for _, id := range []string{"design", "go"} {
				if _, ok := sawSkill[id]; ok {
					ran = append(ran, id)
				}
			}
			if len(ran) != len(tt.wantSkills) {
				t.Fatalf("AI ran skills %v, want %v", ran, tt.wantSkills)
			}
			for i := range ran {
				if ran[i] != tt.wantSkills[i] {
					t.Fatalf("AI ran skills %v, want %v", ran, tt.wantSkills)
				}
			}
			if len(tt.wantFiles) > 0 {
				if got := sawSkill["design"]; len(got) != len(tt.wantFiles) {
					t.Fatalf("design skill saw %v, want %v", got, tt.wantFiles)
				}
			}
			if goFiles := sawSkill["go"]; len(goFiles) > 0 && (len(goFiles) != 1 || goFiles[0] != "main.go") {
				t.Fatalf("go skill saw %v, want only main.go", goFiles)
			}
		})
	}
}
