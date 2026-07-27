// Package ollama adapts any OpenAI-compatible /v1/chat/completions
// endpoint to agentsdk's core.Provider interface. It is stdlib-only
// (net/http + encoding/json + bufio) — no vendor SDK, no third-party
// dependencies beyond the agentsdk core itself.
//
// File layout:
//
//	provider.go   — entry point, Provider struct, interface methods
//	options.go    — functional options for New
//	dto.go        — wire-format types (RequestBody, ChatMessage, ...)
//	validate.go   — RequestBody.Validate()
//	auth_api.go   — ResolveAPIKey / ResolveBaseURL
//	stream.go     — SSE parser → core.ModelChunk
//	models.go     — DefaultCatalog
package ollama

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
)

// Provider implements core.Provider against any OpenAI-compatible
// /chat/completions endpoint. Stream is implemented in-process over
// the SSE protocol when the upstream supports it; fall back to a
// single Generate call when streaming fails.
type Provider struct {
	baseURL string
	auth    core.Auth
	model   string
	client  *http.Client
}

// New constructs a Provider. baseURL defaults to http://localhost:11434/v1
// (local Ollama); apiKey is optional — Ollama is keyless, while LM
// Studio / vLLM / OpenAI all accept a Bearer.
func New(opts ...Option) (*Provider, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return &Provider{
		baseURL: strings.TrimRight(ResolveBaseURL(cfg.baseURL), "/"),
		auth:    core.Auth{APIKey: ResolveAPIKey(cfg.apiKey)},
		model:   cfg.model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Generate implements core.Provider. Sends a blocking request and
// returns the full ModelResult.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	body, err := toRequestBody(req, p.model, false)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("ollama: marshal: %w", err)
	}
	if err := body.Validate(); err != nil {
		return core.ModelResult{}, err
	}
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return core.ModelResult{}, err
	}
	p.applyHeaders(httpReq, req.Auth, false)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("ollama: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("ollama: read: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return core.ModelResult{}, fmt.Errorf("ollama: status %d: %s", resp.StatusCode, string(respBody))
	}
	var cr Response
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return core.ModelResult{}, fmt.Errorf("ollama: decode: %w", err)
	}
	return fromResponse(cr), nil
}

// Stream implements core.StreamProvider. Returns a channel of core.ModelChunk;
// the final chunk carries Done=true.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	body, err := toRequestBody(req, p.model, true)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal: %w", err)
	}
	if err := body.Validate(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	p.applyHeaders(httpReq, req.Auth, true)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: http: %w", err)
	}
	// Body is closed inside ParseStream's goroutine when ctx cancels or
	// the channel drains; the runtime owns the lifecycle here.
	return ParseStream(ctx, resp.Body)
}

// applyHeaders sets Content-Type, optional Accept for SSE, and the
// Bearer token when one was configured. Local Ollama installs leave
// the Authorization header off entirely.
// override is the per-call core.ModelRequest.Auth, merged on top of the
// credential bound at construction; a zero override changes nothing.
func (p *Provider) applyHeaders(req *http.Request, override core.Auth, stream bool) {
	a := p.auth.Merge(override)
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if tok := a.Token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
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

// toRequestBody flattens the core.ModelRequest into the OpenAI wire shape.
func toRequestBody(req core.ModelRequest, model string, stream bool) (RequestBody, error) {
	body := RequestBody{
		Model:     model,
		Messages:  toChatMessages(req.Messages),
		MaxTokens: maxTokensOrDefault(req),
		Stream:    stream,
	}
	if len(req.Tools) > 0 {
		body.Tools = toToolDefs(req.Tools)
	}
	return body, nil
}

// toChatMessages flattens each core.Message into one ChatMessage. We
// fold the parts (text + tool_use + tool_result) into the single
// ChatMessage shape OpenAI expects; the original core.Message may have
// many parts but the wire format only carries one content string.
func toChatMessages(msgs []core.Message) []ChatMessage {
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
		text, toolCalls, toolResults := flattenMessage(m)
		cm := ChatMessage{Role: role, Content: text, ToolCalls: toolCalls}
		if len(toolResults) > 0 {
			cm.ToolCallID = toolResults[0].CallID
			cm.Content = toolResults[0].outputAsString()
			cm.Name = toolResults[0].Name
		}
		out = append(out, cm)
	}
	return out
}

// flatToolResult holds the bits we need to render one tool result in
// the (collapsed) OpenAI wire shape.
type flatToolResult struct {
	CallID string
	Name   string
	Output any
}

// outputAsString renders the tool result's Output as a string for the
// `content` field of a `role=tool` ChatMessage.
func (r flatToolResult) outputAsString() string {
	if s, ok := r.Output.(string); ok {
		return s
	}
	raw, _ := json.Marshal(r.Output)
	return string(raw)
}

// flattenMessage walks the parts of a single core.Message and pulls
// out the bits the OpenAI wire shape cares about: concatenated text,
// assistant-side tool calls, and the first tool result (we collapse
// multiple tool results to the first one — multi-tool flows in
// practice return one tool_result per turn).
func flattenMessage(m core.Message) (string, []ToolCall, []flatToolResult) {
	var sb strings.Builder
	var tcs []ToolCall
	var trs []flatToolResult
	for _, c := range m.Parts {
		switch c.Kind {
		case core.PART_KIND_PLAIN_TEXT:
			sb.WriteString(c.Text)
		case core.PART_KIND_TOOL_USE:
			if c.ToolUse != nil {
				args, _ := json.Marshal(c.ToolUse.Args)
				tc := ToolCall{ID: c.ToolUse.ID, Type: "function"}
				tc.Function.Name = c.ToolUse.Name
				tc.Function.Arguments = string(args)
				tcs = append(tcs, tc)
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
	return sb.String(), tcs, trs
}

// toToolDefs translates the agentsdk tool specs into the OpenAI
// `tools` array shape.
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

// maxTokensOrDefault returns the request's MaxTokens, or 4096 if not set.
func maxTokensOrDefault(req core.ModelRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 4096
}

// ---------------------------------------------------------------------------
// internal — response decoding
// ---------------------------------------------------------------------------

// fromResponse folds a wire Response into the agentsdk ModelResult.
func fromResponse(cr Response) core.ModelResult {
	out := core.ModelResult{
		Usage: core.TokenUsage{
			PromptTokens:     cr.Usage.PromptTokens,
			CompletionTokens: cr.Usage.CompletionTokens,
			TotalTokens:      cr.Usage.TotalTokens,
		},
	}
	for _, c := range cr.Choices {
		out.Text += c.Message.Content
		out.StopReason = c.FinishReason
		for _, tc := range c.Message.ToolCalls {
			var args map[string]any
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			out.ToolCalls = append(out.ToolCalls, core.ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			})
		}
	}
	return out
}
