package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/rules"
)

// maxRuleBody bounds a rule request; the longest rule is about 1.2 KB.
const maxRuleBody = 8 << 10

// RuleService is what the handlers need from the rules use case.
type RuleService interface {
	List(ctx context.Context, userID string) ([]review.Rule, error)
	Create(ctx context.Context, userID string, r review.Rule) (review.Rule, error)
	Update(ctx context.Context, userID, id string, r review.Rule) (review.Rule, error)
	Delete(ctx context.Context, userID, id string) error
}

// WithRules enables the endpoints for a user's own review standards.
func WithRules(svc RuleService) Option { return func(a *API) { a.rules = svc } }

type ruleDTO struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Instructions string `json:"instructions"`
	Severity     string `json:"severity"`
	Enabled      bool   `json:"enabled"`
}

type ruleBody struct {
	Title        string `json:"title"`
	Instructions string `json:"instructions"`
	Severity     string `json:"severity"`
	Enabled      *bool  `json:"enabled"`
}

func toRuleDTO(r review.Rule) ruleDTO {
	return ruleDTO{ID: r.ID, Title: r.Title, Instructions: r.Instructions, Severity: string(r.Severity), Enabled: r.Enabled}
}

// userID returns the signed-in user's id, or answers 401 itself.
func (a *API) userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	user := a.sessionUser(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Sign in to manage your standards.")
		return "", false
	}
	return user.ID, true
}

func (a *API) listRules(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.userID(w, r)
	if !ok {
		return
	}
	rs, err := a.rules.List(r.Context(), uid)
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	out := make([]ruleDTO, 0, len(rs))
	for _, rule := range rs {
		out = append(out, toRuleDTO(rule))
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": out})
}

func (a *API) createRule(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.userID(w, r)
	if !ok {
		return
	}
	rule, ok := readRule(w, r)
	if !ok {
		return
	}
	created, err := a.rules.Create(r.Context(), uid, rule)
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toRuleDTO(created))
}

func (a *API) updateRule(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.userID(w, r)
	if !ok {
		return
	}
	rule, ok := readRule(w, r)
	if !ok {
		return
	}
	updated, err := a.rules.Update(r.Context(), uid, r.PathValue("id"), rule)
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toRuleDTO(updated))
}

func (a *API) deleteRule(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.userID(w, r)
	if !ok {
		return
	}
	if err := a.rules.Delete(r.Context(), uid, r.PathValue("id")); err != nil {
		writeRuleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readRule decodes a rule request body; a missing "enabled" means on.
func readRule(w http.ResponseWriter, r *http.Request) (review.Rule, bool) {
	var body ruleBody
	if apiErr := decodeJSON(w, r, maxRuleBody, &body); apiErr != nil {
		apiErr.write(w)
		return review.Rule{}, false
	}
	enabled := body.Enabled == nil || *body.Enabled
	return review.Rule{Title: body.Title, Instructions: body.Instructions, Severity: review.Severity(body.Severity), Enabled: enabled}, true
}

func writeRuleError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *rules.InvalidError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, "invalid_rule", invalid.Message)
	case errors.Is(err, review.ErrRuleNotFound):
		writeError(w, http.StatusNotFound, "rule_not_found", "That rule does not exist.")
	case errors.Is(err, review.ErrRuleLimit):
		writeError(w, http.StatusConflict, "limit_reached", "You can have at most 20 standards. Delete one to add another.")
	default:
		slog.Error("rules request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong.")
	}
}
