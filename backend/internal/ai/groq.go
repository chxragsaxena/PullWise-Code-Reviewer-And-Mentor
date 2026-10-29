package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// DefaultGroqModel is used unless overridden.
const DefaultGroqModel = "openai/gpt-oss-120b"

const groqDefaultBaseURL = "https://api.groq.com/openai/v1"

type GroqProvider struct {
	apiKey       string
	model        string
	systemPrompt string
	baseURL      string
	http         *http.Client
	sleep        func(ctx context.Context, d time.Duration) error
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
		sleep:        sleepContext,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

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

// maxRateLimitWait is the longest we pause for a rate limit; a longer wait fails the call.
const maxRateLimitWait = 30 * time.Second

// maxRateLimitRetries is how many times a rate-limited call is retried.
const maxRateLimitRetries = 2

var retryInMessage = regexp.MustCompile(`try again in ([0-9.]+)s`)

// do sends the request, waiting out Groq's per-minute rate limit (429) and retrying.
func (p *GroqProvider) do(ctx context.Context, body groqRequest) (*groqResponse, error) {
	for attempt := 0; ; attempt++ {
		resp, wait, err := p.send(ctx, body)
		if wait == 0 || attempt >= maxRateLimitRetries {
			return resp, err
		}
		if wait > maxRateLimitWait {
			return nil, err
		}
		if sleepErr := p.sleep(ctx, wait); sleepErr != nil {
			return nil, sleepErr
		}
	}
}

// retryDelay reads how long Groq asks us to wait: the Retry-After header, else the message text.
func retryDelay(h http.Header, message string) time.Duration {
	if secs, err := strconv.ParseFloat(h.Get("Retry-After"), 64); err == nil && secs > 0 {
		return time.Duration(secs*float64(time.Second)) + 250*time.Millisecond
	}
	if m := retryInMessage.FindStringSubmatch(message); m != nil {
		if secs, err := strconv.ParseFloat(m[1], 64); err == nil && secs > 0 {
			return time.Duration(secs*float64(time.Second)) + 250*time.Millisecond
		}
	}
	return 5 * time.Second
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// send makes one request. wait is non-zero when the call was rate limited and may be retried.
func (p *GroqProvider) send(ctx context.Context, body groqRequest) (result *groqResponse, wait time.Duration, err error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request groq: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var errBody groqErrorBody
		_ = json.Unmarshal(respBody, &errBody) // best-effort; the status code is what matters
		apiErr := &GroqAPIError{StatusCode: resp.StatusCode, Type: errBody.Error.Type, Message: errBody.Error.Message}
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, retryDelay(resp.Header, errBody.Error.Message), apiErr
		}
		return nil, 0, apiErr
	}

	var parsed groqResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, 0, fmt.Errorf("decode response: %w", err)
	}
	return &parsed, 0, nil
}
