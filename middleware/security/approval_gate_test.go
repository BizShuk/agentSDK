package security_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingPolicy struct {
	verdict core.ApprovalAction
	called  bool
}

func (r *recordingPolicy) Decide(_ struct{}, _ core.AutonomyLevel, _ core.CallToolEffect, _ core.ToolSchema) core.ApprovalAction {
	r.called = true
	return r.verdict
}

func TestApprovalGateAllowPassesThrough(t *testing.T) {
	pol := &recordingPolicy{verdict: core.APPROVAL_ACTION_ALLOW}
	mw := security.ApprovalGate(core.AUTONOMY_L2, pol)

	var seen core.Effect
	d := func(_ context.Context, _ core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
		seen = eff
		return core.State{}, nil, false, nil
	}

	_, _, _, err := mw(middleware.Next(d))(context.Background(), core.State{},
		core.Effect{Kind: core.EFFECT_CALL_TOOL, CallTool: &core.CallToolEffect{
			Call: core.ToolCall{ID: "c1", Name: "read", Risk: core.RISK_LEVEL_LOW},
		}})
	require.NoError(t, err)
	assert.Equal(t, core.EFFECT_CALL_TOOL, seen.Kind)
	assert.True(t, pol.called)
}

func TestApprovalGateAskRewritesToRequestApproval(t *testing.T) {
	pol := &recordingPolicy{verdict: core.APPROVAL_ACTION_ASK}
	mw := security.ApprovalGate(core.AUTONOMY_L1, pol)

	var seen core.Effect
	d := func(_ context.Context, _ core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
		seen = eff
		// mimic runtime: REQUEST_APPROVAL sets state to PAUSED_APPROVAL
		s := core.State{Status: core.RUN_STATUS_PAUSED_APPROVAL}
		return s, nil, true, nil
	}

	state, _, term, err := mw(middleware.Next(d))(context.Background(), core.State{},
		core.Effect{Kind: core.EFFECT_CALL_TOOL, CallTool: &core.CallToolEffect{
			Call: core.ToolCall{ID: "c1", Name: "delete_prod", Risk: core.RISK_LEVEL_HIGH},
		}})
	require.NoError(t, err)
	assert.True(t, term, "REQUEST_APPROVAL must terminate the run")
	assert.Equal(t, core.EFFECT_REQUEST_APPROVAL, seen.Kind)
	assert.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, state.Status)
	require.NotNil(t, seen.RequestApproval)
	assert.Equal(t, "delete_prod", seen.RequestApproval.ToolCall.Name)
}

func TestApprovalGateDenyEmitsNotify(t *testing.T) {
	pol := &recordingPolicy{verdict: core.APPROVAL_ACTION_DENY}
	mw := security.ApprovalGate(core.AUTONOMY_L4, pol)

	var seen core.Effect
	d := func(_ context.Context, _ core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
		seen = eff
		return core.State{}, nil, false, nil
	}

	_, _, _, err := mw(middleware.Next(d))(context.Background(), core.State{},
		core.Effect{Kind: core.EFFECT_CALL_TOOL, CallTool: &core.CallToolEffect{
			Call: core.ToolCall{ID: "c1", Name: "delete_prod", Risk: core.RISK_LEVEL_HIGH},
		}})
	require.NoError(t, err)
	assert.Equal(t, core.EFFECT_NOTIFY, seen.Kind)
	assert.Equal(t, "error", seen.Notify.Level)
}

func TestApprovalGateIgnoresNonCallEffects(t *testing.T) {
	pol := &recordingPolicy{verdict: core.APPROVAL_ACTION_ALLOW}
	mw := security.ApprovalGate(core.AUTONOMY_L2, pol)

	called := false
	d := func(_ context.Context, _ core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
		called = true
		return core.State{}, nil, false, nil
	}
	_, _, _, _ = mw(middleware.Next(d))(context.Background(), core.State{},
		core.Effect{Kind: core.EFFECT_CALL_MODEL, CallModel: &core.CallModelEffect{RequestID: "r1"}})
	assert.True(t, called)
	assert.False(t, pol.called, "policy must not be consulted for non-CALL_TOOL")
}