package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// DefaultModel is the model used unless overridden. The user asked for Sonnet
// specifically, which is why this deviates from Opus, the usual default.
const DefaultModel = "claude-sonnet-5"

const reportToolName = "report_findings"

// maxTokens bounds the response. A review's findings list is a handful of
// short entries, not a long document, so this is generous rather than tight.
const maxTokens = 8000

// reportSchema is the input_schema of the report_findings tool. Every object
// sets additionalProperties: false and lists every field as required, per the
// Claude API's rules for strict tool use — see the Common Pitfalls note on
// ToolInputSchemaParam.ExtraFields for why the top-level flag needs that field
// specifically rather than a struct field.
var findingProperties = map[string]any{
	"checkId":     map[string]any{"type": "string", "description": "The checkId this finding belongs to, exactly as given above."},
	"file":        map[string]any{"type": "string"},
	"line":        map[string]any{"type": "integer", "description": "1-based line number, or 0 if the finding applies to the whole file."},
	"endLine":     map[string]any{"type": "integer", "description": "0 for a single line or the whole file."},
	"evidence":    map[string]any{"type": "string", "description": "The exact code that shows the problem."},
	"found":       map[string]any{"type": "string"},
	"why":         map[string]any{"type": "string"},
	"improvement": map[string]any{"type": "string"},
	"confidence":  map[string]any{"type": "number", "description": "Your confidence, 0 to 1, that this is a real, worthwhile finding."},
}

var findingRequired = []string{"checkId", "file", "line", "endLine", "evidence", "found", "why", "improvement", "confidence"}

// reportFindingsTool builds the tool definition. It's a function, not a
// package var, so the ExtraFields map is never shared/mutated across calls.
func reportFindingsTool() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        reportToolName,
		Description: anthropic.String("Report the issues you found in this pull request, one per finding."),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"findings": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"properties":           findingProperties,
						"required":             findingRequired,
						"additionalProperties": false,
					},
				},
			},
			Required:    []string{"findings"},
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}
}

// toolInput is the shape the model's tool call is parsed into.
type toolInput struct {
	Findings []struct {
		CheckID     string  `json:"checkId"`
		File        string  `json:"file"`
		Line        int     `json:"line"`
		EndLine     int     `json:"endLine"`
		Evidence    string  `json:"evidence"`
		Found       string  `json:"found"`
		Why         string  `json:"why"`
		Improvement string  `json:"improvement"`
		Confidence  float64 `json:"confidence"`
	} `json:"findings"`
}

// ClaudeProvider implements Provider using the Anthropic Messages API.
type ClaudeProvider struct {
	client *anthropic.Client
	model  anthropic.Model
	// requestOpts accumulates options (e.g. a test base URL) applied before
	// the client is built in NewClaudeProvider.
	requestOpts []option.RequestOption
}

// ClaudeOption configures a ClaudeProvider.
type ClaudeOption func(*ClaudeProvider)

// WithModel overrides DefaultModel.
func WithModel(model string) ClaudeOption {
	return func(p *ClaudeProvider) { p.model = anthropic.Model(model) }
}

// WithBaseURL points the client at another host (used by tests).
func WithBaseURL(baseURL string) ClaudeOption {
	return func(p *ClaudeProvider) { p.requestOpts = append(p.requestOpts, option.WithBaseURL(baseURL)) }
}

// NewClaudeProvider builds a provider authenticated with apiKey.
func NewClaudeProvider(apiKey string, opts ...ClaudeOption) *ClaudeProvider {
	p := &ClaudeProvider{model: DefaultModel}
	for _, opt := range opts {
		opt(p)
	}
	clientOpts := append([]option.RequestOption{option.WithAPIKey(apiKey)}, p.requestOpts...)
	client := anthropic.NewClient(clientOpts...)
	p.client = &client
	return p
}

// Review asks Claude to evaluate one skill's AI checks against the changed
// files and returns what it reported. It forces the report_findings tool with
// a strict schema, so the response is always well-formed JSON — there is no
// prose to parse.
func (p *ClaudeProvider) Review(ctx context.Context, input ReviewInput) ([]Finding, error) {
	if len(input.Checks) == 0 {
		return nil, nil
	}

	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:      p.model,
		MaxTokens:  maxTokens,
		System:     []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:   []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(buildPrompt(input)))},
		Tools:      []anthropic.ToolUnionParam{{OfTool: toolPtr(reportFindingsTool())}},
		ToolChoice: anthropic.ToolChoiceParamOfTool(reportToolName),
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return nil, fmt.Errorf("claude review (skill %s): %w (status %d, request %s)",
				input.Skill.ID, err, apiErr.StatusCode, apiErr.RequestID)
		}
		return nil, fmt.Errorf("claude review (skill %s): %w", input.Skill.ID, err)
	}

	switch resp.StopReason {
	case anthropic.StopReasonToolUse:
		// expected
	case anthropic.StopReasonMaxTokens:
		return nil, fmt.Errorf("claude review (skill %s): response was cut off at max_tokens", input.Skill.ID)
	case anthropic.StopReasonRefusal:
		return nil, fmt.Errorf("claude review (skill %s): the model declined to respond (category %q)", input.Skill.ID, resp.StopDetails.Category)
	default:
		return nil, fmt.Errorf("claude review (skill %s): unexpected stop reason %q", input.Skill.ID, resp.StopReason)
	}

	for _, block := range resp.Content {
		use, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok || use.Name != reportToolName {
			continue
		}
		var parsed toolInput
		if err := json.Unmarshal(use.Input, &parsed); err != nil {
			return nil, fmt.Errorf("claude review (skill %s): parse tool input: %w", input.Skill.ID, err)
		}
		return toFindings(parsed), nil
	}
	return nil, fmt.Errorf("claude review (skill %s): response had no %s tool call", input.Skill.ID, reportToolName)
}

func toFindings(parsed toolInput) []Finding {
	out := make([]Finding, 0, len(parsed.Findings))
	for _, f := range parsed.Findings {
		confidence := f.Confidence
		if confidence < 0 {
			confidence = 0
		} else if confidence > 1 {
			confidence = 1
		}
		out = append(out, Finding{
			CheckID: f.CheckID, File: f.File, Line: f.Line, EndLine: f.EndLine,
			Evidence: f.Evidence, Found: f.Found, Why: f.Why, Improvement: f.Improvement,
			Confidence: confidence,
		})
	}
	return out
}

func toolPtr(t anthropic.ToolParam) *anthropic.ToolParam { return &t }
