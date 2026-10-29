// Package review holds the domain types shared across the review pipeline.
package review

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrReviewNotFound means no review has the requested id.
	ErrReviewNotFound = errors.New("review not found")
	// ErrFindingNotFound means the review has no finding with the requested id.
	ErrFindingNotFound = errors.New("finding not found")
	// ErrReviewFinished means an update was refused because the review is already terminal.
	ErrReviewFinished = errors.New("review already finished")
)

// Status is where a review is in its lifecycle.
type Status string

const (
	StatusQueued     Status = "queued"
	StatusFetchingPR Status = "fetching_pr"
	StatusAnalyzing  Status = "analyzing"
	StatusReviewing  Status = "reviewing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Terminal reports whether the status can never change again.
func (s Status) Terminal() bool { return s == StatusCompleted || s == StatusFailed }

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusQueued, StatusFetchingPR, StatusAnalyzing, StatusReviewing, StatusCompleted, StatusFailed:
		return true
	}
	return false
}

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
	ID         string // assigned by the store
	SkillID    string
	SkillName  string // filled from the skill definition when read or stored
	CheckID    string
	CheckTitle string // filled from the skill definition when read or stored
	Source     Source
	Severity   Severity
	Confidence float64
	File       string
	// Line is 1-based; 0 means the whole file (null in the API and database).
	Line int
	// EndLine is 0 when the finding is a single line or the whole file.
	EndLine     int
	Evidence    string
	Found       string
	Why         string
	Improvement string
	Lesson      string // lesson id within the skill, empty if none
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
	case f.File == "":
		return fmt.Errorf("finding %s/%s: needs a file", f.SkillID, f.CheckID)
	case f.Line < 0 || f.EndLine < 0 || (f.EndLine != 0 && f.EndLine < f.Line):
		return fmt.Errorf("finding %s/%s: invalid line range %d-%d", f.SkillID, f.CheckID, f.Line, f.EndLine)
	case f.Evidence == "":
		return fmt.Errorf("finding %s/%s: evidence is required", f.SkillID, f.CheckID)
	}
	return nil
}

// Counts is the number of findings per severity.
type Counts struct {
	Critical   int
	High       int
	Medium     int
	Suggestion int
}

// CountFindings tallies findings by severity.
func CountFindings(findings []Finding) Counts {
	var c Counts
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			c.Critical++
		case SeverityHigh:
			c.High++
		case SeverityMedium:
			c.Medium++
		case SeveritySuggestion:
			c.Suggestion++
		}
	}
	return c
}

// PullRequest describes a pull request; only Owner, Repo and Number are known at creation.
type PullRequest struct {
	Owner        string
	Repo         string
	Number       int
	Title        string
	Author       string
	URL          string
	State        string
	HeadSHA      string
	FilesChanged int
	Additions    int
	Deletions    int
}

// ErrorInfo explains why a review failed.
type ErrorInfo struct {
	Code    string
	Message string
}

// Review is one analysis run of a pull request.
type Review struct {
	ID            string
	Status        Status
	StatusMessage string
	StatusAt      time.Time
	PullRequest   PullRequest
	Counts        Counts
	CreatedAt     time.Time
	CompletedAt   *time.Time
	Error         *ErrorInfo
	Findings      []Finding
}

// Event is one status change of a review.
type Event struct {
	Status  Status
	Message string
	At      time.Time
}

// ChangedFile is one file touched by a pull request.
type ChangedFile struct {
	Path     string
	Language string
	Patch    string
	// Content is the file's text at the PR's head commit; empty for deleted or undownloaded files.
	Content string
}

// AnalysisInput is what a StaticAnalyzer receives.
type AnalysisInput struct {
	Files []ChangedFile
}

// StaticAnalyzer produces deterministic findings; severity and titles come from the skill.
type StaticAnalyzer interface {
	Analyze(ctx context.Context, input AnalysisInput) ([]Finding, error)
}
