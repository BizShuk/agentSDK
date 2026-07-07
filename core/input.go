package core

import (
	"context"
	"time"
)

// EventKind discriminates which field of Event carries the value.
type EventKind string

const (
	// EVENT_OBSERVATION — a fresh observation arrived from a perception ObservationSource.
	EVENT_OBSERVATION EventKind = "observation"
	// EVENT_MODEL_REPLY — the LLM produced a finished result.
	EVENT_MODEL_REPLY EventKind = "model_reply"
	// EVENT_TOOL_RESULT — a dispatched tool call returned.
	EVENT_TOOL_RESULT EventKind = "tool_result"
	// EVENT_HUMAN_DECISION — an out-of-band HITL decision arrived.
	EVENT_HUMAN_DECISION EventKind = "human_decision"
	// EVENT_RESUME — a checkpoint-driven resumption with previously-replayed events.
	EVENT_RESUME EventKind = "resume"
)

// Observation is one reading fed into the loop.
//
// (The agent-loop community settled on "observation"; "percept" traces to
// Rosenblatt 1958 and is rarely used in current LLM-agent papers. We
// keep the convention.)
type Observation struct {
	ID         string    `json:"id"`
	Source     string    `json:"source"` // logical source name (e.g. "logfile:/var/log/sys")
	ObservedAt time.Time `json:"observed_at"`
	Payload    any       `json:"payload"` // opaque to core; perception normalizes per source
}

// ToolCall identifies a single tool invocation; stable across replays.
type ToolCall struct {
	ID   string         `json:"id"`   // idempotency key
	Name string         `json:"name"` // tool name
	Args map[string]any `json:"args"` // raw args decoded from LLM
	Risk RiskLevel      `json:"risk,omitempty"`
}

// ToolResult is what a Tool returns — the loop folds it back as an Event.
type ToolResult struct {
	CallID    string `json:"call_id"` // matches ToolCall.ID
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	Output    any    `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
}

// ModelChunk is a single streamed chunk from the model provider.
// Stream is read by runtime — Decide only sees the folded ModelResult.
type ModelChunk struct {
	Kind    PartKind     `json:"kind"`
	Text    string       `json:"text,omitempty"`
	ToolUse *ToolUseChunk `json:"tool_use,omitempty"`
	Done    bool         `json:"done"`
}

// ToolUseChunk is emitted inside a ModelChunk when the model wants a tool call.
type ToolUseChunk struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
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

// Event drives one Decide. Exactly one payload field is set per Event.
type Event struct {
	Kind          EventKind         `json:"kind"`
	Observation   *Observation      `json:"observation,omitempty"`
	ModelResult   *ModelResult      `json:"model_result,omitempty"`
	ToolResult    *ToolResult       `json:"tool_result,omitempty"`
	HumanDecision *ApprovalDecision `json:"human_decision,omitempty"`
	Seq           int               `json:"seq"`
	ReceivedAt    time.Time         `json:"received_at"`
}

// ObservationSource is what core.Decide assumes a perception source looks
// like. The real interfaces live under perception/ — this is just enough
// for Decide / runtime tests to compile without importing it.
type ObservationSource interface {
	Observations(ctx context.Context) <-chan Observation
}
