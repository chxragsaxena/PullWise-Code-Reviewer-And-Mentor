package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ask"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

type fakeAsk struct {
	got    ask.Request
	seen   seenCtx
	answer ask.Answer
	err    error
}

func (f *fakeAsk) Ask(ctx context.Context, req ask.Request) (ask.Answer, error) {
	f.got, f.seen = req, capture(ctx)
	return f.answer, f.err
}

func askAPI(f *fakeAsk, opts ...Option) http.Handler {
	base := []Option{WithCreateRateLimit(0, 0), WithAskRateLimit(0, 0), WithAsk(f),
		WithAuth(signedIn(&fakeAuth{githubToken: "gho_user"}), "http://localhost:5173", false)}
	return New(fakeService{}, append(base, opts...)...).Handler()
}

const goodAsk = `{"repo":"acme/shop","question":"How does retry work?","history":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}`

func TestAskReturnsTheAnswerWithItsSources(t *testing.T) {
	f := &fakeAsk{answer: ask.Answer{
		Text: "It retries [1].", Repo: "acme/shop", IndexedRef: "abc",
		Sources: []ask.Source{{N: 1, Path: "retry.go", StartLine: 3, EndLine: 9, URL: "https://github.com/acme/shop/blob/abc/retry.go#L3-L9", Snippet: "func Retry()"}},
	}}
	rec := requestAs(askAPI(f), "POST", "/api/ask", goodAsk, false)

	var got answerDTO
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if rec.Code != 200 || got.Answer != "It retries [1]." || got.Repo != "acme/shop" || len(got.Sources) != 1 || got.Sources[0].Path != "retry.go" || got.Sources[0].URL == "" {
		t.Fatalf("response = %d %+v", rec.Code, got)
	}
	if f.got.Repo != "acme/shop" || f.got.Question != "How does retry work?" || len(f.got.History) != 2 || f.got.History[1].Role != "assistant" {
		t.Errorf("the service got %+v", f.got)
	}
}

func TestAskOnlyAddsTheUsersGitHubAccessWhenSignedIn(t *testing.T) {
	f := &fakeAsk{}
	h := askAPI(f)

	requestAs(h, "POST", "/api/ask", goodAsk, false)
	if f.seen != (seenCtx{}) {
		t.Errorf("anonymous: service saw %+v, want no viewer and no token", f.seen)
	}
	requestAs(h, "POST", "/api/ask", goodAsk, true)
	if f.seen.viewer != "42" || f.seen.token != "gho_user" {
		t.Errorf("signed in: service saw %+v, want viewer 42 and their GitHub token", f.seen)
	}
}

func TestAskEmptyListsAreArraysNotNull(t *testing.T) {
	rec := requestAs(askAPI(&fakeAsk{answer: ask.Answer{Text: "none"}}), "POST", "/api/ask", goodAsk, false)
	if !strings.Contains(rec.Body.String(), `"sources":[]`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAskErrorsMapToStatusAndCode(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"bad request", &ask.InvalidError{Message: "Ask a question."}, 400, "invalid_request"},
		{"not configured", ask.ErrUnavailable, 503, "ask_unavailable"},
		{"repo missing", ask.ErrRepoNotFound, 404, "repo_not_found"},
		{"no source", ask.ErrNoSource, 422, "no_source"},
		{"review hidden", review.ErrReviewNotFound, 404, "review_not_found"},
		{"github limit", github.ErrRateLimited, 429, "github_rate_limited"},
		{"ai limit", &ai.GroqAPIError{StatusCode: 429}, 429, "ai_busy"},
		{"indexing timed out", context.DeadlineExceeded, 504, "timeout"},
		{"anything else", errors.New("connection refused to 10.0.0.5:5432"), 502, "ask_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := requestAs(askAPI(&fakeAsk{err: tt.err}), "POST", "/api/ask", goodAsk, false)
			wantError(t, rec, tt.wantStatus, tt.wantCode)
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Error("an internal detail leaked into the response")
			}
		})
	}
}

func TestAskBodyProblems(t *testing.T) {
	h := askAPI(&fakeAsk{})
	wantError(t, requestAs(h, "POST", "/api/ask", `not json`, false), http.StatusBadRequest, "invalid_request")
	big := `{"question":"` + strings.Repeat("a", maxAskBody) + `"}`
	wantError(t, requestAs(h, "POST", "/api/ask", big, false), http.StatusRequestEntityTooLarge, "request_too_large")
	wantError(t, requestAs(h, "GET", "/api/ask", "", false), http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestAskIsRateLimitedPerClient(t *testing.T) {
	h := askAPI(&fakeAsk{}, WithAskRateLimit(1, 2))
	for i := 0; i < 2; i++ {
		if rec := requestAs(h, "POST", "/api/ask", goodAsk, false); rec.Code != 200 {
			t.Fatalf("request %d = %d, want 200 within the burst", i+1, rec.Code)
		}
	}
	rec := requestAs(h, "POST", "/api/ask", goodAsk, false)
	wantError(t, rec, http.StatusTooManyRequests, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("a 429 should say when to retry")
	}
}

func TestAskIsNotRoutedUnlessEnabled(t *testing.T) {
	h := New(fakeService{}, WithCreateRateLimit(0, 0)).Handler()
	wantError(t, requestAs(h, "POST", "/api/ask", goodAsk, false), http.StatusNotFound, "not_found")
}
