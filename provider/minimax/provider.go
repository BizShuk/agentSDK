// Package minimax is a stdlib-only HTTP adapter for the minimax
// Anthropic-Messages-compatible API.
//
// minimax operates at https://api.minimax.io/anthropic and exposes a
// /v1/messages endpoint that mirrors Anthropic's wire format exactly.
// Authentication uses `x-api-key: <MINIMAX_API_KEY>` (Anthropic's
// convention), NOT `Authorization: Bearer`. There is no OAuth flow.
//
// File layout:
//
//   - provider.go    — entry point, Provider struct, interface methods
//   - dto.go         — wire-format types (RequestBody, ContentBlock, ...)
//   - validate.go    — RequestBody.Validate()
//   - config.go      — endpoint and environment names
//   - stream.go      — SSE parser → core.ModelChunk
//   - models.go      — DefaultCatalog
package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const defaultModel = "MiniMax-M3"

// Provider implements core.Provider against the minimax Anthropic-compat
// API. We avoid anthropic-sdk-go here because minimax is a thin compat
// surface — stdlib HTTP is enough and keeps the dependency footprint small.
type Provider struct {
	baseURL string
	auth    core.Auth
	model   string
	client  *http.Client
}

// New returns a Provider from registry-resolved construction config.
func New(cfg provider.ResolvedConfig) (*Provider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	return &Provider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		auth:    cfg.Auth,
		model:   cfg.Model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Generate implements core.Provider.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	body, err := toRequestBody(req, p.model)
	if err != nil {
		return core.ModelResult{}, err
	}
	if err := body.Validate(); err != nil {
		return core.ModelResult{}, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("minimax: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return core.ModelResult{}, err
	}
	p.applyHeaders(httpReq, req.Auth, false)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("minimax: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("minimax: read: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return core.ModelResult{}, fmt.Errorf("minimax: status %d: %s", resp.StatusCode, respBody)
	}
	var r Response
	if err := json.Unmarshal(respBody, &r); err != nil {
		return core.ModelResult{}, fmt.Errorf("minimax: decode: %w", err)
	}
	return fromResponse(r), nil
}

// Stream implements core.StreamProvider. Returns a channel of model chunks;
// the terminal chunk carries Done=true.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	body, err := toRequestBody(req, p.model)
	if err != nil {
		return nil, err
	}
	if err := body.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("minimax: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(httpReq, req.Auth, true)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("minimax: http: %w", err)
	}
	// Caller is responsible for closing resp.Body via channel completion;
	// ParseStream drains to EOF.
	return ParseStream(ctx, resp.Body), nil
}

// ---------------------------------------------------------------------------
// internal — auth / headers
// ---------------------------------------------------------------------------

// applyHeaders stamps the outbound request. override is the per-call
// core.ModelRequest.Auth, merged on top of the credential bound at
// construction; a zero override leaves the stored one untouched.
func (p *Provider) applyHeaders(req *http.Request, override core.Auth, stream bool) {
	a := p.auth.Merge(override)
	req.Header.Set("Content-Type", "application/json")
	// minimax is Anthropic-compatible: the credential always rides in
	// x-api-key, whichever class it came from.
	if tok := a.Token(); tok != "" {
		req.Header.Set("x-api-key", tok)
	}
	for k, v := range a.Headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
}

// ---------------------------------------------------------------------------
// internal — body construction
// ---------------------------------------------------------------------------

// toRequestBody translates core.ModelRequest → minimax RequestBody.
// System messages are lifted to the top-level `system` field per the
// Anthropic-Messages convention; user/assistant messages keep their role.
func toRequestBody(req core.ModelRequest, model string) (RequestBody, error) {
	body := RequestBody{
		Model:     model,
		MaxTokens: maxTokensOrDefault(req),
		Stream:    false,
	}
	for _, m := range req.Messages {
		if m.Role == core.ROLE_SYSTEM {
			for _, c := range m.Parts {
				if c.Kind == core.PART_KIND_PLAIN_TEXT {
					body.System += c.Text
				}
			}
			continue
		}
		role := "user"
		if m.Role == core.ROLE_ASSISTANT {
			role = "assistant"
		}
		var blocks []ContentParam
		for _, c := range m.Parts {
			switch c.Kind {
			case core.PART_KIND_PLAIN_TEXT:
				if c.Text != "" {
					blocks = append(blocks, ContentParam{Type: "text", Text: c.Text})
				}
			case core.PART_KIND_REASONING:
				if c.Text != "" || c.Reasoning != nil {
					if c.Reasoning != nil && (c.Reasoning.ID != "" || c.Reasoning.EncryptedContent != "") {
						return RequestBody{}, fmt.Errorf("minimax: reasoning part carries Responses continuation metadata; encode it into an Anthropic signature before calling the adapter")
					}
					block := ContentParam{Type: "thinking", Thinking: c.Text}
					if c.Reasoning != nil {
						block.Signature = c.Reasoning.Signature
					}
					blocks = append(blocks, block)
				}
			case core.PART_KIND_TOOL_USE:
				if c.ToolUse != nil {
					raw, _ := json.Marshal(c.ToolUse.Args)
					blocks = append(blocks, ContentParam{
						Type:  "tool_use",
						ID:    c.ToolUse.ID,
						Name:  c.ToolUse.Name,
						Input: raw,
					})
				}
			case core.PART_KIND_TOOL_RESULT:
				if c.ToolResult != nil {
					raw, _ := json.Marshal(c.ToolResult.Output)
					blocks = append(blocks, ContentParam{
						Type:      "tool_result",
						ToolUseID: c.ToolResult.CallID,
						Content:   raw,
						IsError:   c.ToolResult.Error != "",
					})
				}
			}
		}
		body.Messages = append(body.Messages, MessageParam{Role: role, Content: blocks})
	}
	for _, s := range req.Tools {
		var raw json.RawMessage
		switch v := s.Parameters.(type) {
		case json.RawMessage:
			raw = v
		default:
			if s.Parameters != nil {
				raw, _ = json.Marshal(s.Parameters)
			}
		}
		body.Tools = append(body.Tools, ToolUnionParam{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: raw,
		})
	}
	return body, nil
}

// fromResponse translates a minimax Response → core.ModelResult.
func fromResponse(r Response) core.ModelResult {
	out := core.ModelResult{
		StopReason: r.StopReason,
		Usage: core.TokenUsage{
			PromptTokens:     r.Usage.InputTokens,
			CompletionTokens: r.Usage.OutputTokens,
			TotalTokens:      r.Usage.InputTokens + r.Usage.OutputTokens,
		},
	}
	for _, block := range r.Content {
		switch block.Type {
		case "text":
			out.Parts = append(out.Parts, core.Part{Kind: core.PART_KIND_PLAIN_TEXT, Text: block.Text})
		case "thinking":
			out.Parts = append(out.Parts, core.Part{
				Kind:      core.PART_KIND_REASONING,
				Text:      block.Thinking,
				Reasoning: &core.ReasoningState{Signature: block.Signature},
			})
		case "tool_use":
			if block.ID != "" {
				var argsMap map[string]any
				_ = json.Unmarshal(block.Input, &argsMap)
				call := core.ToolCall{
					ID:   block.ID,
					Name: block.Name,
					Args: argsMap,
				}
				out.Parts = append(out.Parts, core.Part{Kind: core.PART_KIND_TOOL_USE, ToolUse: &call})
			}
		}
	}
	return out.NormalizeContent()
}

// maxTokensOrDefault returns req.MaxTokens or 4096 when unset. The
// Anthropic-Messages API requires max_tokens > 0, so we always send a value.
func maxTokensOrDefault(req core.ModelRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 4096
}
