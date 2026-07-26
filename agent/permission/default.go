package permission

import (
	"github.com/bizshuk/agentsdk/core"
)

// DefaultApprovalPolicy implements the standard autonomy-vs-risk grid:
//
//	           L0        L1          L2          L3          L4
//	low       ASK     ALLOW       ALLOW       ALLOW       ALLOW
//	high      ASK     ASK         ASK         ALLOW       ALLOW
type DefaultApprovalPolicy struct{}

// Decide implements core.ApprovalPolicy for Default mode.
func (DefaultApprovalPolicy) Decide(_ struct{}, autonomy core.AutonomyLevel, _ core.CallToolInstruction, schema core.ToolSpec) core.ApprovalAction {
	risk := schema.Risk
	return gridLookup(autonomy, risk)
}

func gridLookup(autonomy core.AutonomyLevel, risk core.RiskLevel) core.ApprovalAction {
	// L0 — full manual.
	if autonomy == core.AUTONOMY_L0 {
		return core.APPROVAL_ACTION_ASK
	}
	// Low risk: automatic from L1 onwards.
	if risk == core.RISK_LEVEL_LOW {
		return core.APPROVAL_ACTION_ALLOW
	}
	// High risk: ASK until L3.
	if autonomy == core.AUTONOMY_L1 || autonomy == core.AUTONOMY_L2 {
		return core.APPROVAL_ACTION_ASK
	}
	// L3, L4 — high risk is automatic too.
	return core.APPROVAL_ACTION_ALLOW
}
