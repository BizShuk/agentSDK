package core_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubPattern is the minimal ThinkingPattern for testing Step dispatch.
type stubPattern struct {
	kind   core.ThinkingKind
	eff    []core.Effect
	called int
}

func (s *stubPattern) Kind() core.ThinkingKind            { return s.kind }
func (s *stubPattern) Decide(state core.State) (core.State, []core.Effect) {
	s.called++
	return state, s.eff
}

func TestNewStepDispatchesByKind(t *testing.T) {
	react := &stubPattern{kind: core.THINK_REACT, eff: []core.Effect{
		{Kind: core.EFFECT_CALL_MODEL, CallModel: &core.CallModelEffect{RequestID: "r1"}},
	}}
	plans := &stubPattern{kind: core.THINK_PLANNER_EXECUTOR, eff: []core.Effect{
		{Kind: core.EFFECT_DONE},
	}}

	step := core.NewStep(map[core.ThinkingKind]core.ThinkingPattern{
		core.THINK_REACT:             react,
		core.THINK_PLANNER_EXECUTOR:  plans,
	})

	t.Run("react path", func(t *testing.T) {
		state := core.State{ThinkingKind: core.THINK_REACT}
		_, effs := step(state, core.Input{Kind: core.INPUT_KIND_PERCEPT})
		assert.Equal(t, 1, react.called)
		assert.Equal(t, 0, plans.called)
		require.Len(t, effs, 1)
		assert.Equal(t, core.EFFECT_CALL_MODEL, effs[0].Kind)
	})

	t.Run("planner path", func(t *testing.T) {
		state := core.State{ThinkingKind: core.THINK_PLANNER_EXECUTOR}
		out, effs := step(state, core.Input{Kind: core.INPUT_KIND_PERCEPT})
		assert.Equal(t, 1, plans.called)
		require.Len(t, effs, 1)
		assert.Equal(t, core.EFFECT_DONE, effs[0].Kind)
		// stub pattern returns the same state reference, so this is just a smoke check
		assert.Equal(t, core.THINK_PLANNER_EXECUTOR, out.ThinkingKind)
	})

	t.Run("unknown kind surfaces NOTIFY", func(t *testing.T) {
		state := core.State{ThinkingKind: core.ThinkingKind("weird")}
		_, effs := step(state, core.Input{Kind: core.INPUT_KIND_PERCEPT})
		require.Len(t, effs, 1)
		assert.Equal(t, core.EFFECT_NOTIFY, effs[0].Kind)
		require.NotNil(t, effs[0].Notify)
		assert.Equal(t, "error", effs[0].Notify.Level)
	})
}

func TestEffectTaggedUnionJSON(t *testing.T) {
	eff := core.Effect{
		Kind: core.EFFECT_CALL_TOOL,
		CallTool: &core.CallToolEffect{
			Call: core.ToolCall{ID: "c1", Name: "read_log_tail", Args: map[string]any{"n": 10}},
		},
	}
	raw, err := jsonMarshal(eff)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"call_tool"`)
	assert.Contains(t, string(raw), `"c1"`)
}

func TestInputKinds(t *testing.T) {
	// Discriminator strings must be stable; downstream runtimes / CLI depend on them.
	assert.Equal(t, "percept", string(core.INPUT_KIND_PERCEPT))
	assert.Equal(t, "model_result", string(core.INPUT_KIND_MODEL_RESULT))
	assert.Equal(t, "tool_result", string(core.INPUT_KIND_TOOL_RESULT))
	assert.Equal(t, "approval_decision", string(core.INPUT_KIND_APPROVAL_DECISION))
	assert.Equal(t, "resume", string(core.INPUT_KIND_RESUME))
}