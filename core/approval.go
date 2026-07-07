package core

import "time"

// PendingApproval is one open HITL gate. Resolved out-of-band via runtime.Engine.
type PendingApproval struct {
	ID          string           `json:"id"`
	Reason      string           `json:"reason"`
	Risk        RiskLevel        `json:"risk"`
	Summary     string           `json:"summary"`
	ToolCall    *ToolCall        `json:"tool_call,omitempty"`
	RequestedAt time.Time        `json:"requested_at"`
	Decision    ApprovalDecision `json:"decision,omitempty"`
	DecidedAt   *time.Time       `json:"decided_at,omitempty"`
	DecidedBy   string           `json:"decided_by,omitempty"`
}
