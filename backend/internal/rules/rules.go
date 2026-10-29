// Package rules manages the standards a user writes and turns them into a skill for the AI pass.
package rules

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

const (
	// SkillID is the id findings from a user's own rules are stored under.
	SkillID = "custom"
	// SkillName is how those findings are labelled.
	SkillName = "Your standards"

	MaxPerUser      = 20
	MaxTitle        = 120
	MaxInstructions = 1000
)

// InvalidError describes a rule that failed validation.
type InvalidError struct{ Message string }

func (e *InvalidError) Error() string { return e.Message }

// Store is what the service needs from persistence.
type Store interface {
	ListRules(ctx context.Context, userID string) ([]review.Rule, error)
	CreateRule(ctx context.Context, userID string, r review.Rule, max int) (review.Rule, error)
	UpdateRule(ctx context.Context, userID, id string, r review.Rule) (review.Rule, error)
	DeleteRule(ctx context.Context, userID, id string) error
}

// Service validates and stores a user's rules.
type Service struct{ store Store }

// NewService builds a Service over a store.
func NewService(s Store) *Service { return &Service{store: s} }

// List returns the user's rules, oldest first.
func (s *Service) List(ctx context.Context, userID string) ([]review.Rule, error) {
	return s.store.ListRules(ctx, userID)
}

// Create validates and saves a new rule.
func (s *Service) Create(ctx context.Context, userID string, r review.Rule) (review.Rule, error) {
	r, err := Validate(r)
	if err != nil {
		return review.Rule{}, err
	}
	return s.store.CreateRule(ctx, userID, r, MaxPerUser)
}

// Update validates and replaces one of the user's rules.
func (s *Service) Update(ctx context.Context, userID, id string, r review.Rule) (review.Rule, error) {
	r, err := Validate(r)
	if err != nil {
		return review.Rule{}, err
	}
	return s.store.UpdateRule(ctx, userID, id, r)
}

// Delete removes one of the user's rules.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	return s.store.DeleteRule(ctx, userID, id)
}

// Validate trims a rule's text and checks its limits.
func Validate(r review.Rule) (review.Rule, error) {
	r.Title = strings.TrimSpace(r.Title)
	r.Instructions = strings.TrimSpace(r.Instructions)
	if n := utf8.RuneCountInString(r.Title); n < 1 || n > MaxTitle {
		return r, &InvalidError{fmt.Sprintf("The title must be 1 to %d characters.", MaxTitle)}
	}
	if n := utf8.RuneCountInString(r.Instructions); n < 1 || n > MaxInstructions {
		return r, &InvalidError{fmt.Sprintf("The instructions must be 1 to %d characters.", MaxInstructions)}
	}
	if !r.Severity.Valid() {
		return r, &InvalidError{"The severity must be critical, high, medium or suggestion."}
	}
	return r, nil
}

// CheckID is the check id a rule gets inside the custom skill.
func CheckID(ruleID string) string { return "rule-" + ruleID }

// Skill turns a review's rules into an AI-only skill; false if there are none.
func Skill(rs []review.Rule) (skills.Skill, bool) {
	if len(rs) == 0 {
		return skills.Skill{}, false
	}
	checks := make([]skills.Check, 0, len(rs))
	lessons := make(map[string]string, len(rs))
	for _, r := range rs {
		id := CheckID(r.ID)
		checks = append(checks, skills.Check{ID: id, Title: r.Title, Kind: skills.KindAI, Severity: r.Severity, Lesson: id})
		lessons[id] = "**Your rule:** " + r.Instructions
	}
	desc := "Standards written by the user. Report a finding only where the code clearly breaks one of these rules."
	return skills.NewInline(SkillID, SkillName, desc, []string{skills.AnyLanguage}, checks, lessons), true
}

// Placeholder is the skill row findings from custom rules point at.
func Placeholder() skills.Skill {
	return skills.NewInline(SkillID, SkillName, "", []string{skills.AnyLanguage}, nil, nil)
}
