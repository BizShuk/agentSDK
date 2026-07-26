package core

import "time"

// ApprovalDecision is what flows back from a human approver.
type ApprovalDecision string

const (
	APPROVAL_DECISION_APPROVE ApprovalDecision = "approve"
	APPROVAL_DECISION_REJECT  ApprovalDecision = "reject"
	APPROVAL_DECISION_ASK     ApprovalDecision = "ask" // request MORE info — requeue
)

// ApprovalAction describes what the policy decided about one CallTool instruction.
type ApprovalAction int

const (
	APPROVAL_ACTION_ALLOW ApprovalAction = iota // let it through
	APPROVAL_ACTION_DENY                        // reject without surfacing
	APPROVAL_ACTION_ASK                         // surface as RequestApproval instruction
)

// ApprovalPolicy decides whether an instruction needs human sign-off.
// The default implementation lives in agent/permission/default.go and reads
// Autonomy + RiskLevel to choose. Test doubles belong in testutil.
type ApprovalPolicy interface {
	Decide(ctx struct{}, autonomy AutonomyLevel, instruction CallToolInstruction, spec ToolSpec) ApprovalAction
}

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
