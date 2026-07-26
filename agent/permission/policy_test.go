package permission_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/agent/permission"
	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
)

func TestDefaultApprovalPolicy(t *testing.T) {
	p := permission.DefaultApprovalPolicy{}
	low := core.ToolSpec{Risk: core.RISK_LEVEL_LOW}
	high := core.ToolSpec{Risk: core.RISK_LEVEL_HIGH}
	dummyCall := core.CallToolInstruction{}

	// L0 — all ASK
	assert.Equal(t, core.APPROVAL_ACTION_ASK, p.Decide(struct{}{}, core.AUTONOMY_L0, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_ASK, p.Decide(struct{}{}, core.AUTONOMY_L0, dummyCall, high))

	// L1 — low ALLOW, high ASK
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L1, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_ASK, p.Decide(struct{}{}, core.AUTONOMY_L1, dummyCall, high))

	// L2 — low ALLOW, high ASK
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L2, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_ASK, p.Decide(struct{}{}, core.AUTONOMY_L2, dummyCall, high))

	// L3 — low ALLOW, high ALLOW
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L3, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L3, dummyCall, high))

	// L4 — low ALLOW, high ALLOW
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L4, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L4, dummyCall, high))
}

func TestPlanApprovalPolicy(t *testing.T) {
	p := permission.PlanApprovalPolicy{}
	low := core.ToolSpec{Risk: core.RISK_LEVEL_LOW}
	high := core.ToolSpec{Risk: core.RISK_LEVEL_HIGH}
	dummyCall := core.CallToolInstruction{}

	assert.Equal(t, core.APPROVAL_ACTION_ASK, p.Decide(struct{}{}, core.AUTONOMY_L0, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L2, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_DENY, p.Decide(struct{}{}, core.AUTONOMY_L2, dummyCall, high))
}

func TestAcceptEditsApprovalPolicy(t *testing.T) {
	p := permission.AcceptEditsApprovalPolicy{}
	low := core.ToolSpec{Risk: core.RISK_LEVEL_LOW}
	high := core.ToolSpec{Risk: core.RISK_LEVEL_HIGH}
	dummyCall := core.CallToolInstruction{}

	assert.Equal(t, core.APPROVAL_ACTION_ASK, p.Decide(struct{}{}, core.AUTONOMY_L0, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L2, dummyCall, low))
	assert.Equal(t, core.APPROVAL_ACTION_ASK, p.Decide(struct{}{}, core.AUTONOMY_L2, dummyCall, high))
}

func TestBypassApprovalPolicy(t *testing.T) {
	p := permission.BypassApprovalPolicy{}
	high := core.ToolSpec{Risk: core.RISK_LEVEL_HIGH}
	dummyCall := core.CallToolInstruction{}

	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L0, dummyCall, high))
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, p.Decide(struct{}{}, core.AUTONOMY_L2, dummyCall, high))
}

func TestRulesApprovalPolicy(t *testing.T) {
	p := permission.RulesApprovalPolicy{
		Rules: []permission.Rule{
			{Behavior: permission.BEHAVIOR_DENY, Spec: "Bash(rm:*)"},
			{Behavior: permission.BEHAVIOR_ALLOW, Spec: "Bash(git:*)"},
		},
	}

	callGit := core.ToolCall{Name: "Bash", Args: map[string]any{"command": "git status"}}
	action, matched := p.DecideMatch(callGit)
	assert.True(t, matched)
	assert.Equal(t, core.APPROVAL_ACTION_ALLOW, action)

	callRm := core.ToolCall{Name: "Bash", Args: map[string]any{"command": "rm -rf /"}}
	action, matched = p.DecideMatch(callRm)
	assert.True(t, matched)
	assert.Equal(t, core.APPROVAL_ACTION_DENY, action)

	callOther := core.ToolCall{Name: "Edit", Args: map[string]any{"path": "foo.go"}}
	_, matched = p.DecideMatch(callOther)
	assert.False(t, matched)
}
