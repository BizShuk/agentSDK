package action

import (
	"github.com/bizshuk/agentsdk/core"
)

// DefaultApprovalPolicy implements the standard autonomy-vs-risk grid
// described in plans/plan-only-and-plan-breezy-pike.md:
//
//	           L0        L1          L2          L3          L4
//	low       ASK     ALLOW       ALLOW       ALLOW       ALLOW
//	high      ASK     ASK         ASK         ALLOW       ALLOW
//
// L0 = "every action requires explicit human approval" — typical for
// brand-new sessions or when the operator has explicitly dialed trust
// down to zero.
//
// L1 = "low-risk automatic, higher-risk gated" — the enterprise floor.
// L2 = "most automatic, high-risk gated" — the default cap for
//      production runs.
// L3 = "minimal gating" — high-risk no longer requires approval.
// L4 = "fully autonomous" — both low and high risk run unattended.
//
// The policy is purely declarative: it does not inspect the runtime,
// hold state, or fetch external policies. M4 may layer a dynamic
// override (per-tool risk overrides, runtime env flags) on top.
type DefaultApprovalPolicy struct{}

// Decide implements core.ApprovalPolicy.
func (DefaultApprovalPolicy) Decide(_ struct{}, autonomy core.AutonomyLevel, _ core.CallToolInstruction, schema core.ToolSpec) core.ApprovalAction {
	risk := schema.Risk
	return gridLookup(autonomy, risk)
}

// gridLookup returns the verdict for an (autonomy, risk) pair per the
// grid above. Pulled into its own function so tests can target it
// without constructing the full effect / schema pair.
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