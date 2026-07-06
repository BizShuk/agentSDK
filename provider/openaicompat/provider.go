// Package openaicompat is a stdlib-only HTTP adapter for any
// OpenAI-compatible chat-completions endpoint.
//
// Targets include local Ollama (default base URL
// http://localhost:11434/v1), LM Studio, vLLM, and the public
// OpenAI API. Authentication is via the API key; pass an empty
// string for key-less local servers.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// Provider implements core.ModelProvider against any OpenAI-compatible
// /chat/completions endpoint. Stream is implemented in-process over
// the SSE protocol when the upstream supports it; fall back to a
// single Generate call when streaming fails.
type Provider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// New returns a Provider. baseURL defaults to OLLAMA-style local
// host; model defaults to "llama3.2". apiKey defaults to
// OPENAI_API_KEY env, falling back to "" for key-less local hosts.
func New(opts ...Option) (*Provider, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.baseURL == "" {
		cfg.baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if cfg.baseURL == "" {
		cfg.baseURL = "http://localhost:11434/v1"
	}
	if cfg.apiKey == "" {
		cfg.apiKey = os.Getenv("OPENAI_API_KEY")
	}
	return &Provider{
		baseURL: strings.TrimRight(cfg.baseURL, "/"),
		apiKey:  cfg.apiKey,
		model:   cfg.model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}, nil
}

// Name implements core.ModelProvider.
func (p *Provider) Name() string { return "openaicompat:" + p.model }

// Generate implements core.ModelProvider.
func (p *Provider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	body := chatRequest{
		Model:     p.model,
		MaxTokens: maxTokensOrDefault(req),
		Messages:  toOpenAIMessages(req.Messages),
		Stream:    false,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("openaicompat: marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return core.ModelResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("openaicompat: http: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return core.ModelResult{}, fmt.Errorf("openaicompat: read: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return core.ModelResult{}, fmt.Errorf("openaicompat: status %d: %s", resp.StatusCode, string(respBody))
	}
	var cr chatResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return core.ModelResult{}, fmt.Errorf("openaicompat: decode: %w", err)
	}
	return fromOpenAIResponse(cr), nil
}

// Stream implements core.ModelProvider. Streams SSE chunks and
// forwards them as core.ModelChunk.
func (p *Provider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	ch := make(chan core.ModelChunk, 1)
	go func() {
		defer close(ch)
		body := chatRequest{
			Model:    p.model,
			Messages: toOpenAIMessages(req.Messages),
			Stream:   true,
		}
		raw, _ := json.Marshal(body)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(raw))
		if err != nil {
			ch <- core.ModelChunk{Done: true}
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if p.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		resp, err := p.client.Do(httpReq)
		if err != nil {
			ch <- core.ModelChunk{Done: true}
			return
		}
		defer resp.Body.Close()
		// Naive SSE: read each "data: ..." line. End on "data: [DONE]".
		buf := make([]byte, 4096)
		var pending []byte
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				pending = append(pending, buf[:n]...)
				for {
					idx := bytes.IndexByte(pending, '\n')
					if idx < 0 {
						break
					}
					line := string(pending[:idx])
					pending = pending[idx+1:]
					if !strings.HasPrefix(line, "data:") {
						continue
					}
					payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
					if payload == "[DONE]" {
						ch <- core.ModelChunk{Done: true}
						return
					}
					var part streamChunk
					if err := json.Unmarshal([]byte(payload), &part); err != nil {
						continue
					}
					if len(part.Choices) > 0 {
						if delta := part.Choices[0].Delta; delta.Content != "" {
							ch <- core.ModelChunk{Kind: core.CHUNK_KIND_TEXT, Text: delta.Content}
						}
					}
				}
			}
			if err != nil {
				break
			}
		}
		ch <- core.ModelChunk{Done: true}
	}()
	return ch, nil
}

// CountTokens implements core.ModelProvider via chars/4 + 1.
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
// HTTP DTOs (OpenAI-compatible)
// ---------------------------------------------------------------------------

type chatRequest struct {
	Model     string         `json:"model"`
	Messages  []chatMessage  `json:"messages"`
	MaxTokens int            `json:"max_tokens,omitempty"`
	Stream    bool           `json:"stream,omitempty"`
	Tools     []toolDef      `json:"tools,omitempty"`
}

type chatMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content,omitempty"`
	ToolCalls  []toolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Name       string      `json:"name,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
	} `json:"function"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []toolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// ---------------------------------------------------------------------------
// translation
// ---------------------------------------------------------------------------

func toOpenAIMessages(msgs []core.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs))
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
		// Flatten text + tool_use / tool_result into a single message
		// (simple but works for most chat-completions endpoints).
		text, toolCalls, toolResults := flattenMessage(m)
		cm := chatMessage{Role: role, Content: text, ToolCalls: toolCalls}
		if len(toolResults) > 0 {
			cm.ToolCallID = toolResults[0].CallID
			cm.Content = toolResults[0].OutputAsString()
			cm.Name = toolResults[0].Name
		}
		out = append(out, cm)
	}
	return out
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

func flattenMessage(m core.Message) (string, []toolCall, []flatToolResult) {
	var sb strings.Builder
	var tcs []toolCall
	var trs []flatToolResult
	for _, c := range m.Chunks {
		switch c.Kind {
		case core.CHUNK_KIND_TEXT:
			sb.WriteString(c.Text)
		case core.CHUNK_KIND_TOOL_USE:
			if c.ToolUse != nil {
				args, _ := json.Marshal(c.ToolUse.Args)
				tcs = append(tcs, toolCall{
					ID: c.ToolUse.ID, Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: c.ToolUse.Name, Arguments: string(args)},
				})
			}
		case core.CHUNK_KIND_TOOL_RESULT:
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

func toOpenAITools(schemas []core.ToolSchema) []toolDef {
	out := make([]toolDef, 0, len(schemas))
	for _, s := range schemas {
		td := toolDef{Type: "function"}
		td.Function.Name = s.Name
		td.Function.Description = s.Description
		if raw, ok := s.Parameters.(json.RawMessage); ok {
			td.Function.Parameters = raw
		}
		out = append(out, td)
	}
	return out
}

func fromOpenAIResponse(cr chatResponse) core.ModelResult {
	out := core.ModelResult{
		StopReason: "",
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
				ID: tc.ID, Name: tc.Function.Name, Args: args,
			})
		}
	}
	return out
}

func maxTokensOrDefault(req core.ModelRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	return 4096
}