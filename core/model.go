package core

// ModelRequest is what runtime sends to a Provider.
type ModelRequest struct {
	RequestID   string     `json:"request_id,omitempty"`
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
	Kind      PartKind        `json:"kind"`
	Text      string          `json:"text,omitempty"`
	Reasoning *ReasoningState `json:"reasoning,omitempty"`
	ToolUse   *ToolCall       `json:"tool_use,omitempty"`

	// Image carries decoded bytes for a PART_KIND_IMAGE chunk, mirroring
	// Part.Image. Without it a chunk could announce PART_KIND_IMAGE in
	// Kind and carry nothing — which is what happened: an image-
	// generating model streaming its result had the image silently
	// dropped, because the stream vocabulary could not express a value
	// its own Kind enum already named.
	Image     []byte `json:"image,omitempty"`
	ImageMIME string `json:"image_mime,omitempty"`

	Usage TokenUsage `json:"usage,omitzero"`
	Cost  Cost       `json:"cost,omitzero"`
	Done  bool       `json:"done"`
}

// ModelResult is the final, folded result of one model call.
type ModelResult struct {
	// Parts is the canonical ordered assistant content. Text and ToolCalls are
	// compatibility projections for callers written before Parts existed.
	Parts      []Part     `json:"parts,omitempty"`
	Text       string     `json:"text,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"`
	Usage      TokenUsage `json:"usage"`
	Cost       Cost       `json:"cost"`
}

// NormalizeContent makes Parts and the legacy Text/ToolCalls projections
// consistent. When Parts is present it is authoritative; otherwise Parts is
// synthesized from the legacy fields. The returned value owns its Parts slice.
func (r ModelResult) NormalizeContent() ModelResult {
	out := r
	if len(r.Parts) == 0 {
		parts := make([]Part, 0, 1+len(r.ToolCalls))
		if r.Text != "" {
			parts = append(parts, Part{Kind: PART_KIND_PLAIN_TEXT, Text: r.Text})
		}
		for i := range r.ToolCalls {
			call := r.ToolCalls[i]
			parts = append(parts, Part{Kind: PART_KIND_TOOL_USE, ToolUse: &call})
		}
		out.Parts = parts
		return out
	}

	out.Parts = append([]Part(nil), r.Parts...)
	out.Text = ""
	out.ToolCalls = nil
	for i := range out.Parts {
		part := out.Parts[i]
		switch part.Kind {
		case PART_KIND_PLAIN_TEXT:
			out.Text += part.Text
		case PART_KIND_TOOL_USE:
			if part.ToolUse != nil {
				out.ToolCalls = append(out.ToolCalls, *part.ToolUse)
			}
		}
	}
	return out
}

// TokenUsage tracks the billing units reported by a model provider.
type TokenUsage struct {
	// InputTokens is the total input count, including InputCacheReadTokens.
	InputTokens          int `json:"input_tokens"`
	OutputTokens         int `json:"output_tokens"`
	InputCacheReadTokens int `json:"input_cache_read_tokens,omitempty"`
	WebSearchCount       int `json:"web_search_count,omitempty"`
	// TotalTokens is InputTokens + OutputTokens when the provider reports
	// both dimensions.
	TotalTokens int `json:"total_tokens"`
}

// Add returns the component-wise sum of two usage values.
func (u TokenUsage) Add(other TokenUsage) TokenUsage {
	return TokenUsage{
		InputTokens:          u.InputTokens + other.InputTokens,
		OutputTokens:         u.OutputTokens + other.OutputTokens,
		InputCacheReadTokens: u.InputCacheReadTokens + other.InputCacheReadTokens,
		WebSearchCount:       u.WebSearchCount + other.WebSearchCount,
		TotalTokens:          u.TotalTokens + other.TotalTokens,
	}
}
