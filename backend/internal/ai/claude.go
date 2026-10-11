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

// reportFindingsTool builds the tool definition from the shared schema
// (schema.go). It's a function, not a package var, so the ExtraFields map is
// never shared/mutated across calls.
func reportFindingsTool() anthropic.ToolParam {
	schema := findingsObjectSchema()
	return anthropic.ToolParam{
		Name:        reportToolName,
		Description: anthropic.String("Report the issues you found in this pull request, one per finding."),
		Strict:      anthropic.Bool(true),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties:  schema["properties"],
			Required:    []string{"findings"},
			ExtraFields: map[string]any{"additionalProperties": false},
		},
	}
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

func toolPtr(t anthropic.ToolParam) *anthropic.ToolParam { return &t }
