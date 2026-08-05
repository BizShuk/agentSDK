// Package antigravity adapts Google's Antigravity OAuth-backed gateway
// to agentsdk's core.Provider interface.
//
// TODO: confirm the wire format (Anthropic-Messages path) and the live
// gateway endpoint against https://help.router-for-me/configuration/provider/antigravity
// once packet captures exist. The current implementation mirrors the
// Anthropic /v1/messages shape; the gateway may require a different
// path or additional headers (e.g. an Antigravity-specific beta flag).
//
// File layout:
//
//	provider.go    — entry point, Provider struct, interface methods
//	dto.go         — wire-format types (RequestBody, ContentBlock, ...)
//	validate.go    — RequestBody.Validate()
//	config.go      — endpoint and environment names
//	stream.go      — SSE parser → core.ModelChunk
//	models.go      — DefaultCatalog
package antigravity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

const defaultModel = "gemini-3.6-flash-high"

// Provider implements core.Provider against the Antigravity gateway.
type Provider struct {
	baseURL string
	// auth holds whichever credential class the constructor was given.
	// Bearer still outranks APIKey, but the precedence now lives in
	// core.Auth rather than in an if-chain repeated per adapter.
	auth   core.Auth
	model  string
	client *http.Client
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

// Generate implements core.Provider — blocking POST to /v1/messages.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	body, err := buildRequestBody(req, p.model)
	if err != nil {
		return core.ModelResult{}, err
	}
	if err := body.Validate(); err != nil {
		return core.ModelResult{}, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("antigravity: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(raw))
	if err != nil {
		return core.ModelResult{}, err
	}
	p.applyHeaders(httpReq, req.Auth)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("antigravity: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return core.ModelResult{}, fmt.Errorf("antigravity: status %d: %s", resp.StatusCode, respBody)
	}
	var r Response
	if err := json.Unmarshal(respBody, &r); err != nil {
		return core.ModelResult{}, fmt.Errorf("antigravity: decode: %w", err)
	}
	return fromResponse(r), nil
}

// Stream implements core.StreamProvider — SSE POST to /v1/messages.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	body, err := buildRequestBody(req, p.model)
	if err != nil {
		return nil, err
	}
	if err := body.Validate(); err != nil {
		return nil, err
	}
	body.Stream = true
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("antigravity: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	p.applyHeaders(httpReq, req.Auth)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("antigravity: http: %w", err)
	}
	// Caller is responsible for closing resp.Body; Stream is fire-and-forget
	// once ParseStream has the reader.
	ch, _ := ParseStream(ctx, resp.Body)
	return ch, nil
}

// override is the per-call core.ModelRequest.Auth, merged on top of the
// credential bound at construction; a zero override changes nothing.
func (p *Provider) applyHeaders(req *http.Request, override core.Auth) {
	a := p.auth.Merge(override)
	req.Header.Set("Content-Type", "application/json")
	// Anthropic-Messages wire shape carries the API version as a header.
	// Antigravity mirrors Anthropic here — confirm against the gateway
	// docs if a version mismatch surfaces.
	req.Header.Set("anthropic-version", "2023-06-01")
	// Anthropic-style split: OAuth rides in Authorization, an API key in
	// x-api-key. The two are not interchangeable here.
	if a.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+a.Bearer)
	} else if a.APIKey != "" {
		req.Header.Set("x-api-key", a.APIKey)
	}
	for k, v := range a.Headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
}

// ---------------------------------------------------------------------------
// internal — body construction
// ---------------------------------------------------------------------------

func buildRequestBody(req core.ModelRequest, model string) (RequestBody, error) {
	messages, err := toMessageParams(req.Messages)
	if err != nil {
		return RequestBody{}, err
	}
	out := RequestBody{
		Model:     model,
		MaxTokens: maxTokensOrDefault(req),
		Messages:  messages,
	}
	if len(req.Tools) > 0 {
		out.Tools = toToolParams(req.Tools)
	}
	// Collect any system-role messages into the top-level System field.
	// Anthropic-Messages expects `system` outside the messages array.
	var sys strings.Builder
	for _, m := range req.Messages {
		if m.Role != core.ROLE_SYSTEM {
			continue
		}
		for _, p := range m.Parts {
			if p.Kind == core.PART_KIND_PLAIN_TEXT && p.Text != "" {
				if sys.Len() > 0 {
					sys.WriteString("\n\n")
				}
				sys.WriteString(p.Text)
			}
		}
	}
	if sys.Len() > 0 {
		out.System = sys.String()
	}
	return out, nil
}

func maxTokensOrDefault(req core.ModelRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 4096
}

func toMessageParams(msgs []core.Message) ([]MessageParam, error) {
	out := make([]MessageParam, 0, len(msgs))
	for _, m := range msgs {
		// system is hoisted to the top-level System field.
		if m.Role == core.ROLE_SYSTEM {
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
						return nil, fmt.Errorf("antigravity: reasoning part carries Responses continuation metadata; encode it into an Anthropic signature before calling the adapter")
					}
					block := ContentParam{Type: "thinking", Thinking: c.Text}
					if c.Reasoning != nil {
						block.Signature = c.Reasoning.Signature
					}
					blocks = append(blocks, block)
				}
			case core.PART_KIND_IMAGE:
				// core.Part.Image holds raw decoded bytes; the wire wants
				// base64. Dropping the part instead would leave the model
				// answering a vision prompt blind.
				if len(c.Image) > 0 {
					mime := c.ImageMIME
					if mime == "" {
						mime = "image/jpeg"
					}
					blocks = append(blocks, ContentParam{
						Type: "image",
						Source: &ImageSource{
							Type:      "base64",
							MediaType: mime,
							Data:      base64.StdEncoding.EncodeToString(c.Image),
						},
					})
				}
			case core.PART_KIND_TOOL_USE:
				if c.ToolUse != nil {
					input, _ := json.Marshal(c.ToolUse.Args)
					blocks = append(blocks, ContentParam{
						Type:  "tool_use",
						ID:    c.ToolUse.ID,
						Name:  c.ToolUse.Name,
						Input: input,
					})
				}
			case core.PART_KIND_TOOL_RESULT:
				if c.ToolResult != nil {
					payload, _ := json.Marshal(c.ToolResult.Output)
					blocks = append(blocks, ContentParam{
						Type:      "tool_result",
						ToolUseID: c.ToolResult.CallID,
						Content:   payload,
						IsError:   c.ToolResult.Error != "",
					})
				}
			}
		}
		out = append(out, MessageParam{Role: role, Content: blocks})
	}
	return out, nil
}

func toToolParams(specs []core.ToolSpec) []ToolUnionParam {
	out := make([]ToolUnionParam, 0, len(specs))
	for _, s := range specs {
		var schema json.RawMessage
		if raw, ok := s.Parameters.(json.RawMessage); ok && len(raw) > 0 {
			schema = raw
		} else if s.Parameters != nil {
			if m, err := json.Marshal(s.Parameters); err == nil {
				schema = m
			}
		}
		out = append(out, ToolUnionParam{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: schema,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// internal — response folding
// ---------------------------------------------------------------------------

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
