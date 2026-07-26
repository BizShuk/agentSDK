package permission

import (
	"github.com/bizshuk/agentsdk/core"
)

// BypassApprovalPolicy implements the policy for MODE_BYPASS:
// Fully autonomous mode where all tool executions are allowed unconditionally.
type BypassApprovalPolicy struct{}

// Decide implements core.ApprovalPolicy for Bypass mode.
func (BypassApprovalPolicy) Decide(_ struct{}, _ core.AutonomyLevel, _ core.CallToolInstruction, _ core.ToolSpec) core.ApprovalAction {
	return core.APPROVAL_ACTION_ALLOW
}
