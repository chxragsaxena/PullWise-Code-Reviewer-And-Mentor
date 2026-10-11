package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultGroqModel is used unless overridden. gpt-oss-120b supports strict
// JSON-schema structured output (constrained decoding, not best-effort) and
// runs on Groq's hardware at roughly 470-480 tokens/sec — chosen over
// Hugging Face's Inference Providers because Groq's own free tier (14,400
// requests/day, no card) is far more usable than HF's $0.10/month credit;
// see docs/TRACKER.md for the comparison.
const DefaultGroqModel = "openai/gpt-oss-120b"

const groqDefaultBaseURL = "https://api.groq.com/openai/v1"

// GroqProvider implements Provider over Groq's OpenAI-compatible chat
// completions API using plain net/http: the wire format is a handful of
// JSON fields, so a full SDK dependency buys nothing here.
type GroqProvider struct {
	apiKey       string
	model        string
	systemPrompt string
	baseURL      string
	http         *http.Client
}

// GroqOption configures a GroqProvider.
type GroqOption func(*GroqProvider)

// WithGroqModel overrides DefaultGroqModel.
func WithGroqModel(model string) GroqOption { return func(p *GroqProvider) { p.model = model } }

// WithGroqSystemPrompt overrides the reviewer's system prompt (see LoadSystemPrompt).
func WithGroqSystemPrompt(prompt string) GroqOption {
	return func(p *GroqProvider) { p.systemPrompt = prompt }
}

// WithGroqBaseURL points the client at another host (used by tests).
func WithGroqBaseURL(baseURL string) GroqOption { return func(p *GroqProvider) { p.baseURL = baseURL } }

// WithGroqHTTPClient replaces the underlying HTTP client.
func WithGroqHTTPClient(c *http.Client) GroqOption { return func(p *GroqProvider) { p.http = c } }

// NewGroqProvider builds a provider authenticated with apiKey.
func NewGroqProvider(apiKey string, opts ...GroqOption) *GroqProvider {
	p := &GroqProvider{
		apiKey:       apiKey,
		model:        DefaultGroqModel,
		systemPrompt: defaultSystemPrompt,
		baseURL:      groqDefaultBaseURL,
		http:         &http.Client{Timeout: 60 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Wire shapes for the OpenAI-compatible chat completions endpoint. Only the
// fields this provider actually uses are declared.

type groqRequest struct {
	Model          string             `json:"model"`
	Messages       []groqMessage      `json:"messages"`
	MaxTokens      int                `json:"max_tokens"`
	ResponseFormat groqResponseFormat `json:"response_format"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponseFormat struct {
	Type       string         `json:"type"`
	JSONSchema groqJSONSchema `json:"json_schema"`
}

type groqJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type groqResponse struct {
	Choices []struct {
		Message      groqMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
}

type groqErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// GroqAPIError is a non-2xx response from Groq.
type GroqAPIError struct {
	StatusCode int
	Type       string
	Message    string
}

func (e *GroqAPIError) Error() string {
	return fmt.Sprintf("groq: %d %s: %s", e.StatusCode, e.Type, e.Message)
}

// Review asks the model to evaluate one skill's AI checks against the
// changed files, via a strict JSON-schema response format — constrained
// decoding guarantees the output matches the schema, so there is no prose or
// best-effort JSON to recover from.
func (p *GroqProvider) Review(ctx context.Context, input ReviewInput) ([]Finding, error) {
	if len(input.Checks) == 0 {
		return nil, nil
	}

	body := groqRequest{
		Model: p.model,
		Messages: []groqMessage{
			{Role: "system", Content: p.systemPrompt},
			{Role: "user", Content: buildPrompt(input)},
		},
		MaxTokens: maxTokens,
		ResponseFormat: groqResponseFormat{
			Type: "json_schema",
			JSONSchema: groqJSONSchema{
				Name:   reportToolName,
				Strict: true,
				Schema: findingsObjectSchema(),
			},
		},
	}

	resp, err := p.do(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("groq review (skill %s): %w", input.Skill.ID, err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("groq review (skill %s): response had no choices", input.Skill.ID)
	}

	choice := resp.Choices[0]
	switch choice.FinishReason {
	case "stop":
		// expected
	case "length":
		return nil, fmt.Errorf("groq review (skill %s): response was cut off at max_tokens", input.Skill.ID)
	case "content_filter":
		return nil, fmt.Errorf("groq review (skill %s): the response was filtered", input.Skill.ID)
	default:
		return nil, fmt.Errorf("groq review (skill %s): unexpected finish_reason %q", input.Skill.ID, choice.FinishReason)
	}

	var parsed toolInput
	if err := json.Unmarshal([]byte(choice.Message.Content), &parsed); err != nil {
		return nil, fmt.Errorf("groq review (skill %s): parse response JSON: %w", input.Skill.ID, err)
	}
	return toFindings(parsed), nil
}

func (p *GroqProvider) do(ctx context.Context, body groqRequest) (*groqResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request groq: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var errBody groqErrorBody
		_ = json.Unmarshal(respBody, &errBody) // best-effort; the status code is what matters
		return nil, &GroqAPIError{StatusCode: resp.StatusCode, Type: errBody.Error.Type, Message: errBody.Error.Message}
	}

	var result groqResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}
