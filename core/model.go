package core

// ModelSpec is one entry in a provider's catalog. It mirrors pi/ai's Model
// type so picker UIs and budget middleware can plan across providers.
type ModelSpec struct {
	ID            string     `json:"id"`
	Family        string     `json:"family,omitempty"`
	Reasoning     bool       `json:"reasoning,omitempty"`
	Input         []Modality `json:"input,omitempty"`
	ContextWindow int        `json:"context_window,omitempty"`
	MaxTokens     int        `json:"max_tokens,omitempty"`
}

// Modality enumerates input types a model accepts.
type Modality string

const (
	MODALITY_TEXT  Modality = "text"
	MODALITY_IMAGE Modality = "image"
	MODALITY_AUDIO Modality = "audio"
)

// ModelRequest is what runtime sends to a Provider.
type ModelRequest struct {
	Messages    []Message  `json:"messages"`
	Tools       []ToolSpec `json:"tools,omitempty"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	StopReasons []string   `json:"stop_reasons,omitempty"`

	// Auth overrides the provider's built-in credential for this call.
	// Empty value means to use the credential bound at construction time.
	Auth Auth `json:"auth,omitempty"`
}

// ModelChunk is a single streamed chunk from the model provider.
// Stream is read by runtime; Decide only sees the folded ModelResult.
type ModelChunk struct {
	Kind    PartKind  `json:"kind"`
	Text    string    `json:"text,omitempty"`
	ToolUse *ToolCall `json:"tool_use,omitempty"`
	Done    bool      `json:"done"`
}

// ModelResult is the final, folded result of one model call.
type ModelResult struct {
	Text       string     `json:"text,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"`
	Usage      TokenUsage `json:"usage"`
}

// TokenUsage tracks token accounting. Providers report approximate counts.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Add folds usage into a target accounting struct.
func (u TokenUsage) Add() TokenUsage {
	return u // value receiver; arithmetic lives at the call site to avoid double-counting
}
