// Package review holds the domain types shared across the review pipeline.
package review

import (
	"context"
	"fmt"
)

// Severity ranks how serious a finding is.
type Severity string

const (
	SeverityCritical   Severity = "critical"
	SeverityHigh       Severity = "high"
	SeverityMedium     Severity = "medium"
	SeveritySuggestion Severity = "suggestion"
)

// Valid reports whether s is a known severity.
func (s Severity) Valid() bool {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeveritySuggestion:
		return true
	}
	return false
}

// Source says how a finding was produced.
type Source string

const (
	// SourceDeterministic findings come from analyzers and always have confidence 1.0.
	SourceDeterministic Source = "deterministic"
	// SourceAI findings come from an LLM and must cite evidence from the code.
	SourceAI Source = "ai"
)

// Finding is a teaching unit: what was found, why it matters and how to improve it.
type Finding struct {
	SkillID     string
	CheckID     string
	Source      Source
	Severity    Severity
	Confidence  float64
	File        string
	Line        int
	EndLine     int
	Evidence    string
	Found       string
	Why         string
	Improvement string
	Lesson      string
}

// Validate checks the invariants every finding must satisfy before it is stored.
func (f Finding) Validate() error {
	switch {
	case f.SkillID == "" || f.CheckID == "":
		return fmt.Errorf("finding needs a skill and check id")
	case !f.Severity.Valid():
		return fmt.Errorf("finding %s/%s: invalid severity %q", f.SkillID, f.CheckID, f.Severity)
	case f.Source != SourceDeterministic && f.Source != SourceAI:
		return fmt.Errorf("finding %s/%s: invalid source %q", f.SkillID, f.CheckID, f.Source)
	case f.Confidence < 0 || f.Confidence > 1:
		return fmt.Errorf("finding %s/%s: confidence %v out of range", f.SkillID, f.CheckID, f.Confidence)
	case f.Source == SourceDeterministic && f.Confidence != 1:
		return fmt.Errorf("finding %s/%s: deterministic findings must have confidence 1", f.SkillID, f.CheckID)
	case f.File == "" || f.Line < 1:
		return fmt.Errorf("finding %s/%s: needs a file and a line", f.SkillID, f.CheckID)
	case f.Evidence == "":
		return fmt.Errorf("finding %s/%s: evidence is required", f.SkillID, f.CheckID)
	}
	return nil
}

// ChangedFile is one file touched by a pull request.
type ChangedFile struct {
	Path     string
	Language string
	Patch    string
}

// AnalysisInput is what a StaticAnalyzer receives.
type AnalysisInput struct {
	Files []ChangedFile
}

// StaticAnalyzer produces deterministic findings. Semgrep and our Tree-sitter
// checks are separate implementations.
type StaticAnalyzer interface {
	Analyze(ctx context.Context, input AnalysisInput) ([]Finding, error)
}
