// Package grok adapts xAI's Grok API (which exposes an OpenAI-compatible
// /v1/chat/completions endpoint) to agentsdk's core.Provider interface.
//
// This file re-exports the wire-format types under our own namespace so
// callers outside this package do not need to depend on the OpenAI SDK
// directly. The shapes are kept in sync with the public docs at
// https://docs.x.ai/docs and OpenAI's chat-completions reference at
// https://platform.openai.com/docs/api-reference/chat.
package grok

import "encoding/json"

// RequestBody is the POST /v1/chat/completions wire shape we send.
type RequestBody struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
	Stream    bool          `json:"stream,omitempty"`
	Tools     []ToolDef     `json:"tools,omitempty"`
}

// ChatMessage is one element of request.messages.
//
// Tool-calls and tool-results use the OpenAI conventions: assistant
// messages with role=assistant carry an array of tool_calls, and tool
// results arrive as separate messages with role=tool and tool_call_id.
//
// Content is `any` because the spec allows two shapes: a plain string for
// text-only turns, and an array of ContentPart for multimodal ones. Build
// it with textContent / multimodalContent — nothing else is valid on the
// wire, and Validate enforces that before the request leaves.
type ChatMessage struct {
	Role             string     `json:"role"`
	Content          any        `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	Name             string     `json:"name,omitempty"`
}

// ContentPart is one element of a multimodal content array. Type is
// "text" or "image_url"; exactly one payload field is set.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL carries an inline image as an RFC 2397 data URI:
// data:<mime>;base64,<payload>.
type ImageURL struct {
	URL string `json:"url"`
}

// textContent renders a text-only turn as the plain-string form the spec
// prefers.
func textContent(text string) any { return text }

// multimodalContent renders a turn carrying at least one image as the array
// form. Text leads so the instruction precedes the images it refers to.
func multimodalContent(text string, images []ImageURL) any {
	parts := make([]ContentPart, 0, len(images)+1)
	if text != "" {
		parts = append(parts, ContentPart{Type: "text", Text: text})
	}
	for i := range images {
		image := images[i]
		parts = append(parts, ContentPart{Type: "image_url", ImageURL: &image})
	}
	return parts
}

// ToolCall is one element of a chat message's tool_calls array.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolDef is one element of request.tools — the function spec xAI expects.
type ToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
	} `json:"function"`
}

// Response is the non-stream POST /v1/chat/completions response shape.
type Response struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role             string     `json:"role"`
			Content          string     `json:"content"`
			ReasoningContent string     `json:"reasoning_content,omitempty"`
			ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// StreamChunk is one SSE chunk from a streaming POST /v1/chat/completions
// response. The Choices[0].Delta.Content carries the incremental text.
type StreamChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string `json:"role,omitempty"`
			Content          string `json:"content,omitempty"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}
