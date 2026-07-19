package core

// StreamEventKind discriminates StreamEvent payloads.
type StreamEventKind string

const (
	STREAM_RUN_START   StreamEventKind = "run_start"
	STREAM_MESSAGE     StreamEventKind = "message" // folded assistant text for one turn
	STREAM_TOOL_START  StreamEventKind = "tool_start"
	STREAM_TOOL_RESULT StreamEventKind = "tool_result"
	STREAM_RUN_END     StreamEventKind = "run_end"

	// STREAM_MESSAGE_DELTA is reserved for token-level streaming once
	// providers surface native SSE through the engine; the folded
	// STREAM_MESSAGE is always emitted regardless.
	STREAM_MESSAGE_DELTA StreamEventKind = "message_delta"
)

// StreamEvent is the presentation-facing event stream: what a TUI, a
// stream-json writer, or an RPC bridge renders. It is deliberately flat and
// JSON-stable — wire/ and tui/ consume it without importing runtime.
type StreamEvent struct {
	Kind         StreamEventKind `json:"kind"`
	RunID        string          `json:"run_id,omitempty"`
	Turn         int             `json:"turn,omitempty"`
	ContentIndex int             `json:"content_index,omitempty"`
	Text         string          `json:"text,omitempty"`
	ToolCall     *ToolCall       `json:"tool_call,omitempty"`
	ToolResult   *ToolResult     `json:"tool_result,omitempty"`
	Status       RunStatus       `json:"status,omitempty"`
}

// EventSink receives StreamEvents as the run progresses. nil = drop.
// Implementations must not block: the engine calls OnStreamEvent inline.
type EventSink interface {
	OnStreamEvent(ev StreamEvent)
}
