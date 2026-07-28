// Package openaichat owns the OpenAI Chat Completions wire contract shared by
// provider adapters that have proven byte-for-byte compatibility.
package openaichat

import "encoding/json"

type requestBody struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Tools       []toolDef     `json:"tools,omitempty"`
}

// chatMessage is one outbound entry in the request messages array.
//
// Content is `any` because the spec allows two shapes: a plain string for
// text-only turns, and an array of contentPart for multimodal ones. Build
// it with textContent / multimodalContent — nothing else is valid on the
// wire, and validate() enforces that before the request leaves.
type chatMessage struct {
	Role             string     `json:"role"`
	Content          any        `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []toolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	Name             string     `json:"name,omitempty"`
}

// contentPart is one element of a multimodal content array. Type is
// "text" or "image_url"; exactly one payload field is set.
type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL carries an inline image. Ollama, LM Studio, vLLM and OpenAI all
// accept an RFC 2397 data URI here: data:<mime>;base64,<payload>.
type imageURL struct {
	URL string `json:"url"`
}

// responseMessage is the assistant turn decoded back. It is separate from
// chatMessage because responses only ever carry the plain-string content
// form — no server replies with a multimodal array.
type responseMessage struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []toolCall `json:"tool_calls,omitempty"`
}

// textContent renders a text-only turn as the plain-string form the spec
// prefers. Servers that predate multimodal support only parse this shape.
func textContent(text string) any { return text }

// multimodalContent renders a turn carrying at least one image as the array
// form. Text leads so the instruction precedes the images it refers to.
func multimodalContent(text string, images []imageURL) any {
	parts := make([]contentPart, 0, len(images)+1)
	if text != "" {
		parts = append(parts, contentPart{Type: "text", Text: text})
	}
	for i := range images {
		image := images[i]
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &image})
	}
	return parts
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

type response struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int             `json:"index"`
		Message      responseMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type streamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string     `json:"role,omitempty"`
			Content          string     `json:"content,omitempty"`
			ReasoningContent string     `json:"reasoning_content,omitempty"`
			ToolCalls        []toolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}
