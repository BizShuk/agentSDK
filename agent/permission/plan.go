package permission

import (
	"github.com/bizshuk/agentsdk/core"
)

// PlanApprovalPolicy implements the policy for MODE_PLAN:
// Low-risk operations are allowed automatically, while high-risk operations are denied.
type PlanApprovalPolicy struct{}

// Decide implements core.ApprovalPolicy for Plan mode.
func (PlanApprovalPolicy) Decide(_ struct{}, autonomy core.AutonomyLevel, _ core.CallToolInstruction, schema core.ToolSpec) core.ApprovalAction {
	if autonomy == core.AUTONOMY_L0 {
		return core.APPROVAL_ACTION_ASK
	}
	if schema.Risk == core.RISK_LEVEL_LOW {
		return core.APPROVAL_ACTION_ALLOW
	}
	return core.APPROVAL_ACTION_DENY
}
