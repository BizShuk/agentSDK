// Package anthropic adapts the official anthropic-sdk-go to agentsdk's
// core.ModelProvider. The adapter is intentionally thin — it owns the
// auth token + model selection, and translates core.Message ⇄
// anthropic.MessageParam in both directions.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/bizshuk/agentsdk/core"
)

// Provider implements core.ModelProvider against the Anthropic API.
type Provider struct {
	client anthropic.Client
	model  anthropic.Model
}

// New returns a Provider using the ANTHROPIC_API_KEY env var (or the
// pass-in key). model defaults to claude-3-5-sonnet-latest.
func New(opts ...Option) (*Provider, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.apiKey == "" {
		cfg.apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if cfg.apiKey == "" {
		return nil, fmt.Errorf("anthropic: API key not set (use WithAPIKey or ANTHROPIC_API_KEY)")
	}
	clientOpts := []option.RequestOption{option.WithAPIKey(cfg.apiKey)}
	if cfg.baseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(cfg.baseURL))
	}
	client := anthropic.NewClient(clientOpts...)
	return &Provider{client: client, model: cfg.model}, nil
}

// Name implements core.ModelProvider.
func (p *Provider) Name() string { return "anthropic:" + string(p.model) }

// Generate implements core.ModelProvider.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: int64(maxTokensOrDefault(req)),
		Messages:  toAnthropicMessages(req.Messages),
	}
	if len(req.Tools) > 0 {
		params.Tools = toAnthropicTools(req.Tools)
	}
	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return core.ModelResult{}, err
	}
	return fromAnthropicResponse(resp), nil
}

// Stream implements core.ModelProvider.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	ch := make(chan core.ModelChunk, 1)
	go func() {
		defer close(ch)
		mr, err := p.Generate(ctx, req)
		if err != nil {
			ch <- core.ModelChunk{Kind: core.CHUNK_KIND_TEXT, Text: ""}
			return
		}
		if mr.Text != "" {
			ch <- core.ModelChunk{Kind: core.CHUNK_KIND_TEXT, Text: mr.Text}
		}
		ch <- core.ModelChunk{Done: true}
	}()
	return ch, nil
}

// CountTokens implements core.ModelProvider. The SDK does not expose
// a direct count; we approximate via chars/4 + 1 per message.
func (p *Provider) CountTokens(_ context.Context, msgs []core.Message) (int, error) {
	n := 0
	for _, m := range msgs {
		for _, c := range m.Chunks {
			if c.Kind == core.CHUNK_KIND_TEXT {
				n += len(c.Text)/4 + 1
			}
		}
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// translation helpers
// ---------------------------------------------------------------------------

func toAnthropicMessages(msgs []core.Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		role := anthropic.MessageParamRoleUser
		if m.Role == core.ROLE_ASSISTANT {
			role = anthropic.MessageParamRoleAssistant
		}
		var blocks []anthropic.ContentBlockParamUnion
		for _, c := range m.Chunks {
			switch c.Kind {
			case core.CHUNK_KIND_TEXT:
				if c.Text != "" {
					blocks = append(blocks, anthropic.NewTextBlock(c.Text))
				}
			case core.CHUNK_KIND_TOOL_USE:
				if c.ToolUse != nil {
					blocks = append(blocks, anthropic.NewToolUseBlock(c.ToolUse.ID, c.ToolUse.Args, c.ToolUse.Name))
				}
			case core.CHUNK_KIND_TOOL_RESULT:
				if c.ToolResult != nil {
					outStr := stringify(c.ToolResult.Output)
					blocks = append(blocks, anthropic.NewToolResultBlock(c.ToolResult.CallID, outStr, c.ToolResult.Error != ""))
				}
			}
		}
		out = append(out, anthropic.MessageParam{
			Role:    role,
			Content: blocks,
		})
	}
	return out
}

func toAnthropicTools(schemas []core.ToolSchema) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(schemas))
	for _, s := range schemas {
		var inputSchema any
		if raw, ok := s.Parameters.(json.RawMessage); ok && len(raw) > 0 {
			inputSchema = raw
		} else {
			inputSchema = s.Parameters
		}
		out = append(out, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        s.Name,
				Description: anthropic.String(s.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: inputSchema,
				},
			},
		})
	}
	return out
}

func fromAnthropicResponse(resp *anthropic.Message) core.ModelResult {
	out := core.ModelResult{
		StopReason: string(resp.StopReason),
		Usage: core.TokenUsage{
			PromptTokens:     int(resp.Usage.InputTokens),
			CompletionTokens: int(resp.Usage.OutputTokens),
			TotalTokens:      int(resp.Usage.InputTokens + resp.Usage.OutputTokens),
		},
	}
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			out.Text += block.Text
		case "tool_use":
			if block.ID != "" {
				// Anthropic delivers Input as json.RawMessage.
				var argsMap map[string]any
				_ = json.Unmarshal(block.Input, &argsMap)
				out.ToolCalls = append(out.ToolCalls, core.ToolCall{
					ID:   block.ID,
					Name: block.Name,
					Args: argsMap,
				})
			}
		}
	}
	return out
}

// stringify best-effort converts v to its string form for tool_result
// blocks (which require a string payload).
func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

func maxTokensOrDefault(req core.ModelRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 4096
}
