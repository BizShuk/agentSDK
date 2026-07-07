package action_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
)

func TestDefaultPolicyL0AlwaysAsks(t *testing.T) {
	p := action.DefaultApprovalPolicy{}
	for _, risk := range []core.RiskLevel{core.RISK_LEVEL_LOW, core.RISK_LEVEL_HIGH} {
		got := p.Decide(struct{}{}, core.AUTONOMY_L0, core.CallToolInstruction{}, core.ToolSpec{Risk: risk})
		assert.Equal(t, core.APPROVAL_ACTION_ASK, got, "L0 + %s", risk)
	}
}

func TestDefaultPolicyLowRiskAutoFromL1(t *testing.T) {
	p := action.DefaultApprovalPolicy{}
	for _, autonomy := range []core.AutonomyLevel{
		core.AUTONOMY_L1, core.AUTONOMY_L2, core.AUTONOMY_L3, core.AUTONOMY_L4,
	} {
		got := p.Decide(struct{}{}, autonomy, core.CallToolInstruction{}, core.ToolSpec{Risk: core.RISK_LEVEL_LOW})
		assert.Equal(t, core.APPROVAL_ACTION_ALLOW, got, "%s + low", autonomy)
	}
}

func TestDefaultPolicyHighRiskAutoFromL3(t *testing.T) {
	p := action.DefaultApprovalPolicy{}
	for _, autonomy := range []core.AutonomyLevel{core.AUTONOMY_L3, core.AUTONOMY_L4} {
		got := p.Decide(struct{}{}, autonomy, core.CallToolInstruction{}, core.ToolSpec{Risk: core.RISK_LEVEL_HIGH})
		assert.Equal(t, core.APPROVAL_ACTION_ALLOW, got, "%s + high", autonomy)
	}
}

func TestDefaultPolicyHighRiskAskUntilL2(t *testing.T) {
	p := action.DefaultApprovalPolicy{}
	for _, autonomy := range []core.AutonomyLevel{core.AUTONOMY_L1, core.AUTONOMY_L2} {
		got := p.Decide(struct{}{}, autonomy, core.CallToolInstruction{}, core.ToolSpec{Risk: core.RISK_LEVEL_HIGH})
		assert.Equal(t, core.APPROVAL_ACTION_ASK, got, "%s + high", autonomy)
	}
}
