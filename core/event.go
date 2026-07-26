package core

import "time"

// EventKind discriminates which field of Event carries the value.
type EventKind string

const (
	// EVENT_OBSERVATION — a fresh observation arrived from an ObservationSource.
	EVENT_OBSERVATION EventKind = "observation"
	// EVENT_MODEL_REPLY — the LLM produced a finished result.
	EVENT_MODEL_REPLY EventKind = "model_reply"
	// EVENT_TOOL_RESULT — a dispatched tool call returned.
	EVENT_TOOL_RESULT EventKind = "tool_result"
	// EVENT_HUMAN_DECISION — an out-of-band HITL decision arrived.
	EVENT_HUMAN_DECISION EventKind = "human_decision"
	// EVENT_RESUME — a checkpoint-driven resumption with previously replayed events.
	EVENT_RESUME EventKind = "resume"
)

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
