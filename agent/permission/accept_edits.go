package permission

import (
	"github.com/bizshuk/agentsdk/core"
)

// AcceptEditsApprovalPolicy implements the policy for MODE_ACCEPT_EDITS:
// Low-risk operations are allowed automatically, while high-risk operations require explicit approval (ASK).
type AcceptEditsApprovalPolicy struct{}

// Decide implements core.ApprovalPolicy for AcceptEdits mode.
func (AcceptEditsApprovalPolicy) Decide(_ struct{}, autonomy core.AutonomyLevel, _ core.CallToolInstruction, schema core.ToolSpec) core.ApprovalAction {
	if autonomy == core.AUTONOMY_L0 {
		return core.APPROVAL_ACTION_ASK
	}
	if schema.Risk == core.RISK_LEVEL_LOW {
		return core.APPROVAL_ACTION_ALLOW
	}
	return core.APPROVAL_ACTION_ASK
}
