// Package grok adapts xAI's OpenAI-compatible chat and image-generation APIs
// to agentsdk's provider contracts.
//
// File layout:
//
//   - provider.go    — entry point, Provider struct, interface methods
//   - dto.go         — wire-format types (RequestBody, Response, ...)
//   - validate.go    — RequestBody.Validate()
//   - config.go      — endpoint and environment names
//   - stream.go      — SSE parser -> core.ModelChunk
//   - image.go       — OpenAI-compatible image generation
//   - models.go      — DefaultCatalog
//
// xAI supports API-key and OAuth credentials. Both enter through
// provider.ResolvedConfig.Auth and use Bearer Authorization on the wire.
package grok

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

const defaultModel = "grok-3"

// Provider implements model generation, model streaming, and image generation
// against the xAI Grok API.
type Provider struct {
	baseURL string
	// auth holds whichever credential class the constructor was given.
	// Bearer outranks APIKey inside core.Auth.Token, so the OAuth path
	// still wins without a second field to keep in sync.
	auth        core.Auth
	model       string
	imageModel  string
	client      *http.Client
	imageClient *http.Client
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
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		auth:        cfg.Auth,
		model:       cfg.Model,
		imageModel:  DefaultImageModel,
		client:      &http.Client{Timeout: 120 * time.Second},
		imageClient: &http.Client{Timeout: 3 * time.Minute},
	}, nil
}

// Generate implements core.Provider.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	messages, err := toChatMessages(req.Messages)
	if err != nil {
		return core.ModelResult{}, err
	}
	body := RequestBody{
		Model:     p.model,
		MaxTokens: maxTokensOrDefault(req),
		Messages:  messages,
		Stream:    false,
	}
	if len(req.Tools) > 0 {
		body.Tools = toToolDefs(req.Tools)
	}
	if err := body.Validate(); err != nil {
		return core.ModelResult{}, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("grok: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return core.ModelResult{}, err
	}
	p.applyHeaders(httpReq, req.Auth, false)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("grok: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("grok: read: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return core.ModelResult{}, fmt.Errorf("grok: status %d: %s", resp.StatusCode, string(respBody))
	}
	var cr Response
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return core.ModelResult{}, fmt.Errorf("grok: decode: %w", err)
	}
	return fromResponse(cr), nil
}

// Stream implements core.StreamProvider. Streams SSE chunks and forwards them
// as core.ModelChunk.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	messages, err := toChatMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := RequestBody{
		Model:    p.model,
		Messages: messages,
		Stream:   true,
	}
	if len(req.Tools) > 0 {
		body.Tools = toToolDefs(req.Tools)
	}
	if err := body.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("grok: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(httpReq, req.Auth, true)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("grok: http: %w", err)
	}
	return ParseStream(ctx, resp.Body), nil
}

// authHeader returns the Authorization header value. OAuth bearer wins
// over a baked-in API key when both are present, matching the priority
// documented for the anthropic provider.
func (p *Provider) authHeader(override core.Auth) string {
	token := p.auth.Merge(override).Token()
	if token == "" {
		return ""
	}
	return "Bearer " + token
}

func (p *Provider) applyHeaders(req *http.Request, override core.Auth, stream bool) {
	auth := p.auth.Merge(override)
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if token := auth.Token(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range auth.Headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
}

// ---------------------------------------------------------------------------
// internal — DTO translation
// ---------------------------------------------------------------------------

func toChatMessages(msgs []core.Message) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		role := "user"
		switch m.Role {
		case core.ROLE_SYSTEM:
			role = "system"
		case core.ROLE_ASSISTANT:
			role = "assistant"
		case core.ROLE_TOOL:
			role = "tool"
		}
		text, reasoningText, images, toolCalls, toolResults, err := flattenMessage(m)
		if err != nil {
			return nil, err
		}
		cm := ChatMessage{Role: role, Content: textContent(text), ReasoningContent: reasoningText, ToolCalls: toolCalls}
		if len(images) > 0 {
			cm.Content = multimodalContent(text, images)
		}
		if len(toolResults) > 0 {
			cm.ToolCallID = toolResults[0].CallID
			cm.Content = textContent(toolResults[0].OutputAsString())
			cm.Name = toolResults[0].Name
		}
		out = append(out, cm)
	}
	return out, nil
}

type flatToolResult struct {
	CallID string
	Name   string
	Output any
}

func (r flatToolResult) OutputAsString() string {
	if s, ok := r.Output.(string); ok {
		return s
	}
	raw, _ := json.Marshal(r.Output)
	return string(raw)
}

// flattenMessage pulls the bits the wire shape carries out of one message:
// concatenated text, inline images as data URIs, assistant-side tool calls,
// and tool results.
//
// core.Part.Image holds raw decoded bytes, so they are base64-encoded here.
// An empty ImageMIME falls back to image/jpeg. Dropping an image part instead
// would leave the model answering a vision prompt blind, which reads as a bad
// model rather than a missing capability.
func flattenMessage(m core.Message) (string, string, []ImageURL, []ToolCall, []flatToolResult, error) {
	var sb strings.Builder
	var reasoning strings.Builder
	var images []ImageURL
	var tcs []ToolCall
	var trs []flatToolResult
	for _, c := range m.Parts {
		switch c.Kind {
		case core.PART_KIND_PLAIN_TEXT:
			sb.WriteString(c.Text)
		case core.PART_KIND_REASONING:
			if c.Reasoning != nil && (c.Reasoning.ID != "" || c.Reasoning.Signature != "" || c.Reasoning.EncryptedContent != "") {
				return "", "", nil, nil, nil, fmt.Errorf("grok: Chat reasoning_content cannot preserve reasoning continuation metadata")
			}
			reasoning.WriteString(c.Text)
		case core.PART_KIND_IMAGE:
			if len(c.Image) > 0 {
				mime := c.ImageMIME
				if mime == "" {
					mime = "image/jpeg"
				}
				images = append(images, ImageURL{
					URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(c.Image),
				})
			}
		case core.PART_KIND_TOOL_USE:
			if c.ToolUse != nil {
				args, _ := json.Marshal(c.ToolUse.Args)
				tcs = append(tcs, ToolCall{
					ID: c.ToolUse.ID, Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: c.ToolUse.Name, Arguments: string(args)},
				})
			}
		case core.PART_KIND_TOOL_RESULT:
			if c.ToolResult != nil {
				trs = append(trs, flatToolResult{
					CallID: c.ToolResult.CallID,
					Name:   c.ToolResult.Name,
					Output: c.ToolResult.Output,
				})
			}
		}
	}
	return sb.String(), reasoning.String(), images, tcs, trs, nil
}

func toToolDefs(schemas []core.ToolSpec) []ToolDef {
	out := make([]ToolDef, 0, len(schemas))
	for _, s := range schemas {
		td := ToolDef{Type: "function"}
		td.Function.Name = s.Name
		td.Function.Description = s.Description
		if raw, ok := s.Parameters.(json.RawMessage); ok {
			td.Function.Parameters = raw
		}
		out = append(out, td)
	}
	return out
}

func fromResponse(cr Response) core.ModelResult {
	out := core.ModelResult{
		StopReason: "",
		Usage: core.TokenUsage{
			InputTokens:          cr.Usage.InputTokens,
			OutputTokens:         cr.Usage.OutputTokens,
			InputCacheReadTokens: cr.Usage.InputDetails.CachedTokens,
			TotalTokens:          cr.Usage.TotalTokens,
		},
	}
	for _, c := range cr.Choices {
		if c.Message.ReasoningContent != "" {
			out.Parts = append(out.Parts, core.Part{
				Kind: core.PART_KIND_REASONING,
				Text: c.Message.ReasoningContent,
			})
		}
		if c.Message.Content != "" {
			out.Parts = append(out.Parts, core.Part{
				Kind: core.PART_KIND_PLAIN_TEXT,
				Text: c.Message.Content,
			})
		}
		out.StopReason = c.FinishReason
		for _, tc := range c.Message.ToolCalls {
			var args map[string]any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			call := core.ToolCall{
				ID: tc.ID, Name: tc.Function.Name, Args: args,
			}
			out.Parts = append(out.Parts, core.Part{Kind: core.PART_KIND_TOOL_USE, ToolUse: &call})
		}
	}
	return out.NormalizeContent()
}

func maxTokensOrDefault(req core.ModelRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 4096
}
