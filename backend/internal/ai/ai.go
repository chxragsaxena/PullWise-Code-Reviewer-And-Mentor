// Package ai runs the judgment-based checks a skill marks kind: ai, behind a Provider interface.
package ai

import (
	"context"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

// Finding is one issue the model reported.
type Finding struct {
	CheckID string
	File    string
	// Line is 1-based; 0 means the finding applies to the whole file.
	Line int
	// EndLine is 0 for a single line or the whole file.
	EndLine     int
	Evidence    string
	Found       string
	Why         string
	Improvement string
	// Confidence is 0..1: how sure the model is that this is a real issue.
	Confidence float64
}

type ContextSnippet struct {
	Path      string
	StartLine int
	EndLine   int
	Content   string
}

// ReviewInput is one skill's AI checks, applied to one review's changed files.
type ReviewInput struct {
	Skill   skills.Skill
	Checks  []skills.Check // the subset of Skill.Checks with Kind == KindAI
	Files   []review.ChangedFile
	Context []ContextSnippet
}

// Provider evaluates a skill's AI checks against a set of changed files.
type Provider interface {
	Review(ctx context.Context, input ReviewInput) ([]Finding, error)
}
