// Package cli defines the on-the-wire shape of an agentsdk run.
//
// Envelope is the JSONL line emitted by the runtime — every effect
// that the operator might want to see (percept, assistant, tool_call,
// tool_result, approval_request, approval_decision, checkpoint,
// result, error) maps to a distinct Type.
//
// Codec translates Envelopes to / from JSONL bytes so external tooling
// (dashboards, tailers, replayers) can consume runs without depending
// on the SDK's internal types.
package cli

import (
	"encoding/json"
	"time"
)

// MessageType discriminates the 9 envelope kinds emitted by a run.
type MessageType string

const (
	MSG_TYPE_OBSERVATION          MessageType = "observation"
	MSG_TYPE_ASSISTANT            MessageType = "assistant"
	MSG_TYPE_TOOL_CALL            MessageType = "tool_call"
	MSG_TYPE_TOOL_RESULT          MessageType = "tool_result"
	MSG_TYPE_APPROVAL_REQUEST     MessageType = "approval_request"
	MSG_TYPE_HUMAN_DECISION       MessageType = "human_decision"
	MSG_TYPE_CHECKPOINT           MessageType = "checkpoint"
	MSG_TYPE_RESULT               MessageType = "result"
	MSG_TYPE_ERROR                MessageType = "error"
)

// Envelope is one JSONL line on the wire. Exactly one of the payload
// pointers is non-nil per Envelope.
type Envelope struct {
	Type          MessageType          `json:"type"`
	RunID         string               `json:"run_id,omitempty"`
	Turn          int                  `json:"turn,omitempty"`
	Timestamp     time.Time            `json:"ts"`
	Observation   *ObservationPayload  `json:"observation,omitempty"`
	Assistant     *AssistantPayload    `json:"assistant,omitempty"`
	ToolCall      *ToolCallPayload     `json:"tool_call,omitempty"`
	ToolResult    *ToolResultPayload   `json:"tool_result,omitempty"`
	Approval      *ApprovalPayload     `json:"approval,omitempty"`
	Decision      *DecisionPayload     `json:"decision,omitempty"`
	Checkpoint    *CheckpointPayload   `json:"checkpoint,omitempty"`
	Result        *ResultPayload       `json:"result,omitempty"`
	Error         *ErrorPayload        `json:"error,omitempty"`
}

// ObservationPayload mirrors core.Observation but uses only JSON-friendly fields.
type ObservationPayload struct {
	ID         string    `json:"id"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
	Payload    any       `json:"payload"`
}

// AssistantPayload represents an LLM reply (text + tool_calls).
type AssistantPayload struct {
	Text       string          `json:"text,omitempty"`
	ToolCalls  []ToolCallLite  `json:"tool_calls,omitempty"`
	StopReason string          `json:"stop_reason,omitempty"`
	Usage      TokenUsageLite  `json:"usage,omitempty"`
}

// ToolCallLite is a stripped-down tool call entry — the LLM-friendly
// view (just the id, name, and args). Full risk metadata lives on
// the runtime side.
type ToolCallLite struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Args  map[string]any `json:"args"`
}

// TokenUsageLite is the prompt/completion/total token tally.
type TokenUsageLite struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolCallPayload mirrors core.CallToolEffect for the CLI stream.
type ToolCallPayload struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Args  map[string]any `json:"args"`
	Risk  string         `json:"risk,omitempty"`
}

// ToolResultPayload mirrors core.ToolResult.
type ToolResultPayload struct {
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Output  any    `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	ElapsedMS int64 `json:"elapsed_ms,omitempty"`
}

// ApprovalPayload is the operator-facing view of a PendingApproval.
type ApprovalPayload struct {
	ID      string    `json:"id"`
	Reason  string    `json:"reason"`
	Risk    string    `json:"risk"`
	Summary string    `json:"summary"`
	ToolCall *ToolCallPayload `json:"tool_call,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
}

// DecisionPayload is the human's verdict on a PendingApproval.
type DecisionPayload struct {
	ApprovalID string    `json:"approval_id"`
	Decision   string    `json:"decision"`
	DecidedBy  string    `json:"decided_by,omitempty"`
	DecidedAt  time.Time `json:"decided_at"`
}

// CheckpointPayload is the persistence marker — emitted before the
// runtime saves State so a tailer can correlate WAL entries with
// snapshots.
type CheckpointPayload struct {
	RunID string `json:"run_id"`
	Turn  int    `json:"turn"`
	Reason string `json:"reason,omitempty"`
}

// ResultPayload is the terminal envelope for a completed run.
type ResultPayload struct {
	Status string `json:"status"`
	Turn   int    `json:"turn"`
}

// ErrorPayload carries a non-recoverable runtime error.
type ErrorPayload struct {
	Message string `json:"message"`
	Kind    string `json:"kind,omitempty"` // "budget" | "model" | "approval_rejected"
}

// MarshalJSON encodes the envelope. Override to strip zero timestamps.
func (e Envelope) MarshalJSON() ([]byte, error) {
	type alias Envelope
	a := alias(e)
	if a.Timestamp.IsZero() {
		a.Timestamp = time.Now().UTC()
	}
	return json.Marshal(a)
}