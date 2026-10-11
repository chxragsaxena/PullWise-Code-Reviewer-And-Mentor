package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func fakeGroqServer(t *testing.T, respond func(body map[string]any) (int, string)) (*httptest.Server, *GroqProvider) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		status, payload := respond(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, payload)
	}))
	t.Cleanup(srv.Close)
	return srv, NewGroqProvider("test-key", WithGroqBaseURL(srv.URL))
}

func groqChatResponse(finishReason, content string) string {
	data, _ := json.Marshal(content)
	return fmt.Sprintf(`{
		"id": "chatcmpl_1", "object": "chat.completion", "model": %q,
		"choices": [{"index": 0, "message": {"role": "assistant", "content": %s}, "finish_reason": %q}],
		"usage": {"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150}
	}`, DefaultGroqModel, string(data), finishReason)
}

func TestGroqReviewParsesFindingsFromStructuredOutput(t *testing.T) {
	sk := testSkill(t)
	var gotBody map[string]any
	_, p := fakeGroqServer(t, func(body map[string]any) (int, string) {
		gotBody = body
		return http.StatusOK, groqChatResponse("stop", `{
			"findings": [
				{"checkId": "goroutine-leak-risk", "file": "worker.go", "line": 12, "endLine": 18,
				 "evidence": "go func() { <-ch }()", "found": "no way to stop", "why": "leaks forever",
				 "improvement": "add a context", "confidence": 0.82}
			]
		}`)
	})

	findings, err := p.Review(context.Background(), ReviewInput{
		Skill: sk, Checks: sk.Checks,
		Files: []review.ChangedFile{{Path: "worker.go", Patch: "go func() { <-ch }()"}},
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %v", findings)
	}
	f := findings[0]
	if f.CheckID != "goroutine-leak-risk" || f.File != "worker.go" || f.Line != 12 || f.EndLine != 18 || f.Confidence != 0.82 {
		t.Fatalf("finding = %+v", f)
	}

	if gotBody["model"] != DefaultGroqModel {
		t.Errorf("model = %v, want %v", gotBody["model"], DefaultGroqModel)
	}
	rf, _ := gotBody["response_format"].(map[string]any)
	if rf["type"] != "json_schema" {
		t.Fatalf("response_format = %v, want type json_schema", rf)
	}
	js, _ := rf["json_schema"].(map[string]any)
	if js["strict"] != true || js["name"] != reportToolName {
		t.Errorf("json_schema = %v, want strict:true and name %q", js, reportToolName)
	}
	schema, _ := js["schema"].(map[string]any)
	if schema["additionalProperties"] != false {
		t.Errorf("schema.additionalProperties = %v, want false", schema["additionalProperties"])
	}
	messages, _ := gotBody["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %v, want a system and a user message", messages)
	}
}

func TestGroqReviewWithNoChecksSkipsTheCallEntirely(t *testing.T) {
	called := false
	_, p := fakeGroqServer(t, func(map[string]any) (int, string) {
		called = true
		return 200, groqChatResponse("stop", `{"findings":[]}`)
	})

	findings, err := p.Review(context.Background(), ReviewInput{Skill: testSkill(t), Checks: nil})
	if err != nil || findings != nil || called {
		t.Fatalf("findings=%v err=%v called=%v; want a no-op with no request sent", findings, err, called)
	}
}

func TestGroqReviewEmptyFindingsIsNotAnError(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeGroqServer(t, func(map[string]any) (int, string) { return 200, groqChatResponse("stop", `{"findings":[]}`) })
	findings, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err != nil || len(findings) != 0 {
		t.Fatalf("findings=%v err=%v; want zero findings, no error", findings, err)
	}
}

func TestGroqReviewClampsOutOfRangeConfidence(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeGroqServer(t, func(map[string]any) (int, string) {
		return 200, groqChatResponse("stop", `{"findings":[
			{"checkId":"c","file":"a.go","line":1,"endLine":0,"evidence":"x","found":"f","why":"w","improvement":"i","confidence":1.4},
			{"checkId":"c","file":"a.go","line":2,"endLine":0,"evidence":"x","found":"f","why":"w","improvement":"i","confidence":-0.3}
		]}`)
	})
	findings, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err != nil || len(findings) != 2 || findings[0].Confidence != 1 || findings[1].Confidence != 0 {
		t.Fatalf("findings = %+v, err = %v", findings, err)
	}
}

func TestGroqReviewHandlesAPIErrorWithoutLeakingInternals(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeGroqServer(t, func(map[string]any) (int, string) {
		return http.StatusTooManyRequests, `{"error":{"message":"rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}`
	})
	_, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err == nil || !strings.Contains(err.Error(), sk.ID) {
		t.Fatalf("error = %v, want it to name the skill", err)
	}
	var apiErr *GroqAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
		t.Fatalf("error = %v, want a *GroqAPIError with status 429", err)
	}
}

func TestGroqReviewLengthFinishReasonIsAnError(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeGroqServer(t, func(map[string]any) (int, string) { return 200, groqChatResponse("length", `{"findings":[`) })
	_, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err == nil || !strings.Contains(err.Error(), "max_tokens") {
		t.Fatalf("error = %v, want it to mention max_tokens", err)
	}
}

func TestGroqReviewNoChoicesIsAnError(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeGroqServer(t, func(map[string]any) (int, string) {
		return 200, `{"id":"c","object":"chat.completion","model":"m","choices":[],"usage":{}}`
	})
	_, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err == nil {
		t.Fatal("expected an error when the response has no choices")
	}
}

func TestGroqReviewMalformedContentIsAnError(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeGroqServer(t, func(map[string]any) (int, string) {
		return 200, groqChatResponse("stop", `not json`)
	})
	_, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err == nil {
		t.Fatal("expected an error when the content is not valid JSON")
	}
}

func TestGroqAPIErrorMessage(t *testing.T) {
	err := &GroqAPIError{StatusCode: 429, Type: "rate_limit_error", Message: "slow down"}
	if got := err.Error(); !strings.Contains(got, "429") || !strings.Contains(got, "slow down") {
		t.Fatalf("Error() = %q", got)
	}
}
