package core

import (
	"context"
	"time"
)

// InputKind discriminates which field of Input carries the value.
type InputKind string

const (
	// INPUT_KIND_PERCEPT — a fresh observation arrived from a perception Source.
	INPUT_KIND_PERCEPT InputKind = "percept"
	// INPUT_KIND_MODEL_RESULT — the LLM produced a finished result.
	INPUT_KIND_MODEL_RESULT InputKind = "model_result"
	// INPUT_KIND_TOOL_RESULT — a dispatched tool call returned.
	INPUT_KIND_TOOL_RESULT InputKind = "tool_result"
	// INPUT_KIND_APPROVAL_DECISION — an out-of-band HITL decision arrived.
	INPUT_KIND_APPROVAL_DECISION InputKind = "approval_decision"
	// INPUT_KIND_RESUME — a checkpoint-driven resumption with previously-replayed inputs.
	INPUT_KIND_RESUME InputKind = "resume"
)

// Percept is one observation fed into the loop.
type Percept struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"` // logical source name (e.g. "logfile:/var/log/sys")
	ObservedAt time.Time `json:"observed_at"`
	Payload   any       `json:"payload"` // opaque to core; perception normalize per source
}

// ToolCall identifies a single tool invocation; stable across replays.
type ToolCall struct {
	ID    string         `json:"id"`            // idempotency key
	Name  string         `json:"name"`          // tool name
	Args  map[string]any `json:"args"`          // raw args decoded from LLM
	Risk  RiskLevel      `json:"risk,omitempty"` // captured at dispatch
}

// ToolResult is what a Tool returns — the loop folds it back as an Input.
type ToolResult struct {
	CallID  string `json:"call_id"` // matches ToolCall.ID
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Output  any    `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	ElapsedMS int64 `json:"elapsed_ms,omitempty"`
}

// ModelChunk is a single streamed chunk from the model provider.
// Stream is read by runtime — Step only sees the folded ModelResult.
type ModelChunk struct {
	Kind   ChunkKind `json:"kind"`
	Text   string    `json:"text,omitempty"`
	ToolUse *ToolUseChunk `json:"tool_use,omitempty"`
	Done   bool      `json:"done"`
}

// ToolUseChunk is emitted inside a ModelChunk when the model wants a tool call.
type ToolUseChunk struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Args  map[string]any `json:"args"`
}

// ModelResult is the final, folded result of one model call.
type ModelResult struct {
	Text       string     `json:"text,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"` // end_turn | tool_use | max_tokens | error
	Usage      TokenUsage `json:"usage"`
}

// TokenUsage tracks token accounting. Providers report approximate counts;
// openaicompat may fall back to chars/4 heuristic.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Add folds usage into a target accounting struct.
func (u TokenUsage) Add() TokenUsage {
	return u // value receiver; arithmetic lives at the call site to avoid double-counting
}

// Input drives one Step. Exactly one payload field is set per Input.
type Input struct {
	Kind             InputKind          `json:"kind"`
	Percept          *Percept           `json:"percept,omitempty"`
	ModelResult      *ModelResult       `json:"model_result,omitempty"`
	ToolResult       *ToolResult        `json:"tool_result,omitempty"`
	ApprovalDecision *ApprovalDecision  `json:"approval_decision,omitempty"`
	Seq              int                `json:"seq"`
	ReceivedAt       time.Time          `json:"received_at"`
}

// Source is what core.Step assumes a perception source looks like.
// The real interfaces live under perception/ — this is just enough for
// Step / runtime tests to compile without importing it.
type Source interface {
	Percepts(ctx context.Context) <-chan Percept
}
