package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/rules"
)

// fakeRules wraps the real rules service over an in-memory store.
type memRules struct {
	next  int
	rules map[string][]review.Rule // user -> rules
}

func (m *memRules) ListRules(_ context.Context, uid string) ([]review.Rule, error) {
	return m.rules[uid], nil
}

func (m *memRules) CreateRule(_ context.Context, uid string, r review.Rule, max int) (review.Rule, error) {
	if len(m.rules[uid]) >= max {
		return review.Rule{}, review.ErrRuleLimit
	}
	m.next++
	r.ID = strconv.Itoa(m.next)
	m.rules[uid] = append(m.rules[uid], r)
	return r, nil
}

func (m *memRules) UpdateRule(_ context.Context, uid, id string, r review.Rule) (review.Rule, error) {
	for i, have := range m.rules[uid] {
		if have.ID == id {
			r.ID = id
			m.rules[uid][i] = r
			return r, nil
		}
	}
	return review.Rule{}, review.ErrRuleNotFound
}

func (m *memRules) DeleteRule(_ context.Context, uid, id string) error {
	for i, have := range m.rules[uid] {
		if have.ID == id {
			m.rules[uid] = append(m.rules[uid][:i], m.rules[uid][i+1:]...)
			return nil
		}
	}
	return review.ErrRuleNotFound
}

func rulesAPI(t *testing.T) (http.Handler, *memRules) {
	t.Helper()
	mem := &memRules{rules: map[string][]review.Rule{}}
	h := New(fakeService{}, WithCreateRateLimit(0, 0),
		WithAuth(signedIn(&fakeAuth{}), "http://localhost:5173", false),
		WithRules(rules.NewService(mem))).Handler()
	return h, mem
}

const goodRule = `{"title":"Wrap errors","instructions":"Use fmt.Errorf with %w.","severity":"high"}`

func TestRulesEndpointsNeedASignedInUser(t *testing.T) {
	h, _ := rulesAPI(t)
	for _, tt := range []struct{ method, path, body string }{
		{"GET", "/api/rules", ""},
		{"POST", "/api/rules", goodRule},
		{"PUT", "/api/rules/1", goodRule},
		{"DELETE", "/api/rules/1", ""},
	} {
		rec := requestAs(h, tt.method, tt.path, tt.body, false)
		wantError(t, rec, http.StatusUnauthorized, "unauthorized")
	}
}

func TestRulesAreNotRoutedUnlessEnabled(t *testing.T) {
	h := New(fakeService{}, WithCreateRateLimit(0, 0), WithAuth(signedIn(&fakeAuth{}), "http://x", false)).Handler()
	wantError(t, requestAs(h, "GET", "/api/rules", "", true), http.StatusNotFound, "not_found")
}

func TestCreateListUpdateDeleteARule(t *testing.T) {
	h, _ := rulesAPI(t)

	rec := requestAs(h, "POST", "/api/rules", goodRule, true)
	var created ruleDTO
	_ = json.NewDecoder(rec.Body).Decode(&created)
	if rec.Code != http.StatusCreated || created.ID == "" || created.Title != "Wrap errors" || created.Severity != "high" || !created.Enabled {
		t.Fatalf("create = %d %+v; a missing \"enabled\" should mean on", rec.Code, created)
	}

	rec = requestAs(h, "GET", "/api/rules", "", true)
	var list struct{ Rules []ruleDTO }
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if rec.Code != 200 || len(list.Rules) != 1 || list.Rules[0].ID != created.ID {
		t.Fatalf("list = %d %+v", rec.Code, list)
	}

	rec = requestAs(h, "PUT", "/api/rules/"+created.ID, `{"title":"Wrap errors v2","instructions":"x","severity":"suggestion","enabled":false}`, true)
	var updated ruleDTO
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if rec.Code != 200 || updated.Title != "Wrap errors v2" || updated.Enabled || updated.Severity != "suggestion" {
		t.Fatalf("update = %d %+v", rec.Code, updated)
	}

	if rec := requestAs(h, "DELETE", "/api/rules/"+created.ID, "", true); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := requestAs(h, "DELETE", "/api/rules/"+created.ID, "", true); rec.Code != http.StatusNotFound {
		t.Fatalf("deleting twice = %d, want 404", rec.Code)
	}
}

func TestEmptyListIsAnEmptyArrayNotNull(t *testing.T) {
	h, _ := rulesAPI(t)
	rec := requestAs(h, "GET", "/api/rules", "", true)
	if !strings.Contains(rec.Body.String(), `"rules":[]`) {
		t.Fatalf("body = %s, want an empty array", rec.Body.String())
	}
}

func TestRuleValidationErrors(t *testing.T) {
	h, _ := rulesAPI(t)
	for name, body := range map[string]string{
		"missing title":    `{"title":"","instructions":"x","severity":"high"}`,
		"bad severity":     `{"title":"t","instructions":"x","severity":"urgent"}`,
		"long title":       `{"title":"` + strings.Repeat("a", rules.MaxTitle+1) + `","instructions":"x","severity":"high"}`,
		"long instruction": `{"title":"t","instructions":"` + strings.Repeat("a", rules.MaxInstructions+1) + `","severity":"high"}`,
	} {
		rec := requestAs(h, "POST", "/api/rules", body, true)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
			continue
		}
		wantError(t, rec, http.StatusBadRequest, "invalid_rule")
	}
	wantError(t, requestAs(h, "POST", "/api/rules", `not json`, true), http.StatusBadRequest, "invalid_request")
}

func TestTheRuleLimitIsA409(t *testing.T) {
	h, mem := rulesAPI(t)
	for i := 0; i < rules.MaxPerUser; i++ {
		mem.rules["42"] = append(mem.rules["42"], review.Rule{ID: strconv.Itoa(100 + i)})
	}
	wantError(t, requestAs(h, "POST", "/api/rules", goodRule, true), http.StatusConflict, "limit_reached")
}

func TestAnotherUsersRuleLooksLikeAMissingOne(t *testing.T) {
	h, mem := rulesAPI(t)
	mem.rules["99"] = []review.Rule{{ID: "5", Title: "someone else's", Severity: review.SeverityHigh}}

	wantError(t, requestAs(h, "PUT", "/api/rules/5", goodRule, true), http.StatusNotFound, "rule_not_found")
	wantError(t, requestAs(h, "DELETE", "/api/rules/5", "", true), http.StatusNotFound, "rule_not_found")
	if len(mem.rules["99"]) != 1 {
		t.Fatal("another user's rule was modified")
	}
}
