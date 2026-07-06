package security_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxMWAllowsAllowedCall(t *testing.T) {
	policy := action.DefaultPolicy()
	mw := security.Sandbox(policy)

	var last core.Effect
	d := func(_ context.Context, _ core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
		last = eff
		return core.State{}, nil, false, nil
	}

	toolCall := core.ToolCall{ID: "c1", Name: "read_file", Args: map[string]any{"path": "/tmp/x"}}
	_, _, _, err := mw(middleware.Next(d))(context.Background(), core.State{},
		core.Effect{Kind: core.EFFECT_CALL_TOOL, CallTool: &core.CallToolEffect{Call: toolCall}})
	require.NoError(t, err)
	assert.Equal(t, core.EFFECT_CALL_TOOL, last.Kind, "allowed call must pass through unchanged")
}

func TestSandboxMWDeniesDisallowedCall(t *testing.T) {
	policy := action.DefaultPolicy()
	mw := security.Sandbox(policy)

	var observed []core.Effect
	d := func(_ context.Context, _ core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
		observed = append(observed, eff)
		if eff.Kind == core.EFFECT_DONE {
			return core.State{}, nil, true, nil
		}
		return core.State{}, nil, false, nil
	}

	toolCall := core.ToolCall{ID: "c1", Name: "read_file", Args: map[string]any{"path": "/etc/passwd"}}
	state, _, _, err := mw(middleware.Next(d))(context.Background(), core.State{},
		core.Effect{Kind: core.EFFECT_CALL_TOOL, CallTool: &core.CallToolEffect{Call: toolCall}})
	require.NoError(t, err)
	require.NotEmpty(t, observed)
	assert.Equal(t, core.EFFECT_NOTIFY, observed[0].Kind, "first dispatched effect must be NOTIFY (denial)")
	require.NotNil(t, observed[0].Notify)
	assert.Equal(t, "error", observed[0].Notify.Level)
	assert.Contains(t, observed[0].Notify.Message, "sandbox denied")
	assert.Equal(t, core.EFFECT_DONE, observed[1].Kind, "second dispatched effect must be DONE")

	// State passes through unchanged in the denied path (we didn't actually call the tool).
	_ = state
}

func TestSandboxMWNonCallEffectsUntouched(t *testing.T) {
	policy := action.DefaultPolicy()
	mw := security.Sandbox(policy)

	var seen core.Effect
	d := func(_ context.Context, _ core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
		seen = eff
		return core.State{}, nil, false, nil
	}

	_, _, _, err := mw(middleware.Next(d))(context.Background(), core.State{},
		core.Effect{Kind: core.EFFECT_CALL_MODEL, CallModel: &core.CallModelEffect{RequestID: "r1"}})
	require.NoError(t, err)
	assert.Equal(t, core.EFFECT_CALL_MODEL, seen.Kind)
}

func TestSandboxMWDeniesDangerousCommand(t *testing.T) {
	policy := action.DefaultPolicy()
	mw := security.Sandbox(policy)

	var observed []core.Effect
	d := func(_ context.Context, _ core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
		observed = append(observed, eff)
		if eff.Kind == core.EFFECT_DONE {
			return core.State{}, nil, true, nil
		}
		return core.State{}, nil, false, nil
	}

	toolCall := core.ToolCall{ID: "c1", Name: "shell", Args: map[string]any{"command": "rm -rf /"}}
	_, _, _, err := mw(middleware.Next(d))(context.Background(), core.State{},
		core.Effect{Kind: core.EFFECT_CALL_TOOL, CallTool: &core.CallToolEffect{Call: toolCall}})
	require.NoError(t, err)
	assert.Equal(t, core.EFFECT_NOTIFY, observed[0].Kind)
}