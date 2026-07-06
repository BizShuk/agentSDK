package planning_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReactFirstThinkEmitsCallModel(t *testing.T) {
	p := planning.NewReAct()
	s := core.State{ThinkingKind: core.THINK_REACT, Messages: []core.Message{
		{Role: core.ROLE_USER, Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "watch log"}}},
	}}
	out, effs := p.Decide(s)
	require.Len(t, effs, 1)
	assert.Equal(t, core.EFFECT_CALL_MODEL, effs[0].Kind)
	assert.Equal(t, planning.REACT_PHASE_ACT, scratchGet(out, planning.REACT_PHASE))
}

func TestReactActEmitsCallTool(t *testing.T) {
	p := planning.NewReAct()
	s := core.State{ThinkingKind: core.THINK_REACT}
	planning.SeedAct(&s, core.ToolCall{ID: "c1", Name: "read_log_tail", Args: map[string]any{"n": 5}})

	_, effs := p.Decide(s)
	require.Len(t, effs, 1)
	assert.Equal(t, core.EFFECT_CALL_TOOL, effs[0].Kind)
	require.NotNil(t, effs[0].CallTool)
	assert.Equal(t, "c1", effs[0].CallTool.Call.ID)
	assert.Equal(t, "read_log_tail", effs[0].CallTool.Call.Name)
}

func TestReactObserveEmitsCallModel(t *testing.T) {
	p := planning.NewReAct()
	s := core.State{ThinkingKind: core.THINK_REACT, Scratch: map[string]any{
		planning.REACT_PHASE: planning.REACT_PHASE_OBSERVE,
	}}
	_, effs := p.Decide(s)
	require.Len(t, effs, 1)
	assert.Equal(t, core.EFFECT_CALL_MODEL, effs[0].Kind)
}

func TestPlannerExecutorSkipsToExecuteWhenBlueprintSeeded(t *testing.T) {
	p := planning.NewPlannerExecutor()
	s := core.State{ThinkingKind: core.THINK_PLANNER_EXECUTOR}
	planning.SeedBlueprint(&s, []core.ToolCall{
		{ID: "s1", Name: "add_todo", Args: map[string]any{"title": "investigate"}},
		{ID: "s2", Name: "add_todo", Args: map[string]any{"title": "fix"}},
	})

	out, effs := p.Decide(s)
	require.Len(t, effs, 1)
	assert.Equal(t, core.EFFECT_CALL_TOOL, effs[0].Kind)
	assert.Equal(t, "s1", effs[0].CallTool.Call.ID)
	assert.Equal(t, 1, scratchGetInt(out, planning.PE_STEP_INDEX))
	assert.Equal(t, planning.PE_PHASE_EXEC, scratchGet(out, planning.PE_PHASE))
}

func TestPlannerExecutorEmitsDONEWhenBlueprintExhausted(t *testing.T) {
	p := planning.NewPlannerExecutor()
	s := core.State{ThinkingKind: core.THINK_PLANNER_EXECUTOR, Scratch: map[string]any{
		planning.PE_PHASE:       planning.PE_PHASE_EXEC,
		planning.PE_STEP_INDEX:  2,
		planning.PE_BLUEPRINT_STEPS: []core.ToolCall{
			{ID: "s1", Name: "a"},
			{ID: "s2", Name: "b"},
		},
	}}
	_, effs := p.Decide(s)
	require.Len(t, effs, 1)
	assert.Equal(t, core.EFFECT_DONE, effs[0].Kind)
}

func TestExecutorCriticOKCritiqueEmitsDONE(t *testing.T) {
	p := planning.NewExecutorCritic()
	s := core.State{ThinkingKind: core.THINK_EXECUTOR_CRITIC}
	planning.SeedCritiqueOK(&s, "looks good")

	_, effs := p.Decide(s)
	require.Len(t, effs, 1)
	assert.Equal(t, core.EFFECT_DONE, effs[0].Kind)
}

func TestExecutorCriticRejectCritiqueEmitsCallModel(t *testing.T) {
	p := planning.NewExecutorCritic()
	s := core.State{ThinkingKind: core.THINK_EXECUTOR_CRITIC}
	planning.SeedCritiqueReject(&s, "step missing precondition")

	out, effs := p.Decide(s)
	require.Len(t, effs, 1)
	assert.Equal(t, core.EFFECT_CALL_MODEL, effs[0].Kind)
	assert.Equal(t, planning.EC_PHASE_EXEC, scratchGet(out, planning.EC_PHASE))
	assert.Equal(t, 1, scratchGetInt(out, planning.EC_ITER))
}

func TestStubPatternsDoNotPanic(t *testing.T) {
	tests := []struct {
		name    string
		pattern core.ThinkingPattern
		kind    core.ThinkingKind
	}{
		{"cot_singleshot", planning.NewCOTSingleshot(), core.THINK_COT_SINGLESHOT},
		{"reflexion", planning.NewReflexion(), core.THINK_REFLEXION},
		{"router", planning.NewRouter(), core.THINK_ROUTER},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.kind, tc.pattern.Kind())
			s := core.State{ThinkingKind: tc.kind}
			out, effs := tc.pattern.Decide(s)
			assert.NotEmpty(t, effs, "stub must return at least one effect")
			// every stub must include a DONE so the run can terminate
			hasDone := false
			for _, e := range effs {
				if e.Kind == core.EFFECT_DONE {
					hasDone = true
				}
			}
			assert.True(t, hasDone, "stub must emit DONE")
			_ = out
		})
	}
}

func scratchGet(s core.State, key string) string {
	if s.Scratch == nil {
		return ""
	}
	v, ok := s.Scratch[key]
	if !ok {
		return ""
	}
	if x, ok := v.(string); ok {
		return x
	}
	return ""
}

func scratchGetInt(s core.State, key string) int {
	if s.Scratch == nil {
		return 0
	}
	v, ok := s.Scratch[key]
	if !ok {
		return 0
	}
	if x, ok := v.(int); ok {
		return x
	}
	return 0
}