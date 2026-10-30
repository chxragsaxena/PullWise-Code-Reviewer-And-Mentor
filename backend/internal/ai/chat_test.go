package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func chatServer(t *testing.T, finish, content string, got *map[string]any) *GroqProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(got)
		data, _ := json.Marshal(content)
		fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%s},"finish_reason":%q}]}`, data, finish)
	}))
	t.Cleanup(srv.Close)
	return NewGroqProvider("k", WithGroqBaseURL(srv.URL))
}

func TestChatSendsTheConversationAsPlainTextWithoutAJSONSchema(t *testing.T) {
	var body map[string]any
	p := chatServer(t, "stop", "  It retries three times [1].  ", &body)

	answer, err := p.Chat(context.Background(), "be brief", []Message{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "follow-up"},
	})
	if err != nil || answer != "It retries three times [1]." {
		t.Fatalf("Chat = %q, %v", answer, err)
	}

	if _, has := body["response_format"]; has {
		t.Error("a chat answer must not request structured JSON output")
	}
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("sent %d messages, want system + 3 turns", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	last, _ := msgs[3].(map[string]any)
	if first["role"] != "system" || first["content"] != "be brief" || last["content"] != "follow-up" {
		t.Errorf("messages = %v", msgs)
	}
}

func TestChatKeepsATruncatedAnswerButRejectsOddEndings(t *testing.T) {
	var body map[string]any
	if got, err := chatServer(t, "length", "partial answer", &body).Chat(context.Background(), "s", nil); err != nil || got != "partial answer" {
		t.Errorf("a length-limited answer = %q, %v; want it kept", got, err)
	}
	if _, err := chatServer(t, "content_filter", "x", &body).Chat(context.Background(), "s", nil); err == nil {
		t.Error("a filtered answer must be an error")
	}
	if _, err := chatServer(t, "stop", "   ", &body).Chat(context.Background(), "s", nil); err == nil {
		t.Error("an empty answer must be an error")
	}
}

func TestReviewStillRequestsAJSONSchema(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprint(w, okReview)
	}))
	t.Cleanup(srv.Close)
	if err := reviewOnce(NewGroqProvider("k", WithGroqBaseURL(srv.URL)), t); err != nil {
		t.Fatal(err)
	}
	rf, _ := body["response_format"].(map[string]any)
	if rf["type"] != "json_schema" {
		t.Fatalf("response_format = %v, want the structured schema for reviews", rf)
	}
}

func TestPromptsFallBackToTheBuiltInWhenTheFileIsMissingOrBlank(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"missing": {},
		"blank":   {askPromptFile: {Data: []byte("  \n ")}},
	} {
		got, err := LoadAskPrompt(fsys)
		if err != nil || !strings.Contains(got, "numbered code") {
			t.Errorf("%s: LoadAskPrompt = %q, %v; want the built-in prompt", name, got, err)
		}
	}
	got, err := LoadAskPrompt(fstest.MapFS{askPromptFile: {Data: []byte("custom prompt\n")}})
	if err != nil || got != "custom prompt" {
		t.Errorf("LoadAskPrompt = %q, %v; want the file's text", got, err)
	}
	if sys, _ := LoadSystemPrompt(fstest.MapFS{}); !strings.Contains(sys, "code reviewer") {
		t.Error("the review prompt fallback was lost")
	}
}
