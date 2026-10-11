// Package ai runs the contextual, judgment-based checks a skill marks
// kind: ai — the ones evidence-based static analysis can't decide on its own
// ("is this abstraction unnecessarily complicated?"). It sits behind the
// Provider interface so the model or vendor can change without touching the
// review pipeline, the same way StaticAnalyzer keeps analyzers swappable.
package ai

import (
	"context"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

// Finding is one issue the model reported. Unlike review.Finding, it carries
// no severity, source or lesson: those come from the skill's check definition
// once the service normalizes the result, exactly as for deterministic
// analyzers. Confidence is the model's own, since AI judgment genuinely varies
// in a way a deterministic check's never does.
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

// ReviewInput is one skill's AI checks, applied to one review's changed files.
type ReviewInput struct {
	Skill  skills.Skill
	Checks []skills.Check // the subset of Skill.Checks with Kind == KindAI
	Files  []review.ChangedFile
}

// Provider evaluates a skill's AI checks against a set of changed files.
type Provider interface {
	Review(ctx context.Context, input ReviewInput) ([]Finding, error)
}
