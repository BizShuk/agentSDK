package core

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
// The default implementation lives in action/approval_policy.go and reads
// Autonomy + RiskLevel to choose. Test doubles belong in testutil.
type ApprovalPolicy interface {
	Decide(ctx struct{}, autonomy AutonomyLevel, eff CallToolInstruction, schema ToolSpec) ApprovalAction
}
