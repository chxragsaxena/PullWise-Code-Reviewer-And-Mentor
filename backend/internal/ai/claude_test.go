package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

func fakeAnthropicServer(t *testing.T, respond func(body map[string]any) (int, string)) (*httptest.Server, *ClaudeProvider) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	return srv, NewClaudeProvider("test-key", WithBaseURL(srv.URL))
}

func toolUseResponse(input string) string {
	return fmt.Sprintf(`{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-5",
		"content": [{"type": "tool_use", "id": "toolu_1", "name": %q, "input": %s}],
		"stop_reason": "tool_use", "stop_sequence": null,
		"usage": {"input_tokens": 100, "output_tokens": 50}
	}`, reportToolName, input)
}

func TestReviewParsesFindingsFromTheToolCall(t *testing.T) {
	sk := testSkill(t)
	var gotBody map[string]any
	srv, p := fakeAnthropicServer(t, func(body map[string]any) (int, string) {
		gotBody = body
		return http.StatusOK, toolUseResponse(`{
			"findings": [
				{"checkId": "goroutine-leak-risk", "file": "worker.go", "line": 12, "endLine": 18,
				 "evidence": "go func() { <-ch }()", "found": "no way to stop", "why": "leaks forever",
				 "improvement": "add a context", "confidence": 0.82}
			]
		}`)
	})
	_ = srv

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

	if gotBody["model"] != DefaultModel {
		t.Errorf("model = %v, want %v", gotBody["model"], DefaultModel)
	}
	toolChoice, _ := gotBody["tool_choice"].(map[string]any)
	if toolChoice["type"] != "tool" || toolChoice["name"] != reportToolName {
		t.Errorf("tool_choice = %v, want a forced call to %s", toolChoice, reportToolName)
	}
	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v, want exactly the report_findings tool", tools)
	}
	tool := tools[0].(map[string]any)
	if tool["strict"] != true {
		t.Errorf("tool.strict = %v, want true", tool["strict"])
	}
	schema := tool["input_schema"].(map[string]any)
	if schema["additionalProperties"] != false {
		t.Errorf("input_schema.additionalProperties = %v, want false", schema["additionalProperties"])
	}
}

func TestReviewWithNoChecksSkipsTheCallEntirely(t *testing.T) {
	called := false
	srv, p := fakeAnthropicServer(t, func(map[string]any) (int, string) {
		called = true
		return 200, toolUseResponse(`{"findings":[]}`)
	})
	defer srv.Close()

	findings, err := p.Review(context.Background(), ReviewInput{Skill: testSkill(t), Checks: nil})
	if err != nil || findings != nil || called {
		t.Fatalf("findings=%v err=%v called=%v; want a no-op with no request sent", findings, err, called)
	}
}

func TestReviewEmptyFindingsIsNotAnError(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeAnthropicServer(t, func(map[string]any) (int, string) { return 200, toolUseResponse(`{"findings":[]}`) })
	findings, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err != nil || len(findings) != 0 {
		t.Fatalf("findings=%v err=%v; want zero findings, no error", findings, err)
	}
}

func TestReviewClampsOutOfRangeConfidence(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeAnthropicServer(t, func(map[string]any) (int, string) {
		return 200, toolUseResponse(`{"findings":[
			{"checkId":"c","file":"a.go","line":1,"endLine":0,"evidence":"x","found":"f","why":"w","improvement":"i","confidence":1.4},
			{"checkId":"c","file":"a.go","line":2,"endLine":0,"evidence":"x","found":"f","why":"w","improvement":"i","confidence":-0.3}
		]}`)
	})
	findings, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err != nil || len(findings) != 2 || findings[0].Confidence != 1 || findings[1].Confidence != 0 {
		t.Fatalf("findings = %+v, err = %v", findings, err)
	}
}

func TestReviewHandlesAPIErrorWithoutLeakingInternals(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeAnthropicServer(t, func(map[string]any) (int, string) {
		return http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`
	})
	_, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err == nil || !strings.Contains(err.Error(), sk.ID) {
		t.Fatalf("error = %v, want it to name the skill", err)
	}
}

func TestReviewMaxTokensIsAnError(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeAnthropicServer(t, func(map[string]any) (int, string) {
		return 200, `{"id":"m","type":"message","role":"assistant","model":"claude-sonnet-5",
			"content":[{"type":"text","text":"cut off"}],"stop_reason":"max_tokens","stop_sequence":null,
			"usage":{"input_tokens":1,"output_tokens":1}}`
	})
	_, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err == nil || !strings.Contains(err.Error(), "max_tokens") {
		t.Fatalf("error = %v, want it to mention max_tokens", err)
	}
}

func TestReviewRefusalIsAnError(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeAnthropicServer(t, func(map[string]any) (int, string) {
		return 200, `{"id":"m","type":"message","role":"assistant","model":"claude-sonnet-5",
			"content":[],"stop_reason":"refusal",
			"stop_details":{"type":"refusal","category":"cyber","explanation":"no"},
			"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
	})
	_, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("error = %v, want it to say the model declined", err)
	}
}

func TestReviewMissingToolCallIsAnError(t *testing.T) {
	sk := testSkill(t)
	_, p := fakeAnthropicServer(t, func(map[string]any) (int, string) {
		return 200, `{"id":"m","type":"message","role":"assistant","model":"claude-sonnet-5",
			"content":[{"type":"text","text":"oops, no tool call"}],"stop_reason":"end_turn","stop_sequence":null,
			"usage":{"input_tokens":1,"output_tokens":1}}`
	})
	_, err := p.Review(context.Background(), ReviewInput{Skill: sk, Checks: sk.Checks})
	if err == nil {
		t.Fatal("expected an error when the model never calls the tool")
	}
}
