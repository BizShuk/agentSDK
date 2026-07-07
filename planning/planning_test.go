package planning_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThinkThenActFirstReasonEmitsCallModel(t *testing.T) {
	p := planning.NewThinkThenAct()
	s := core.State{ReasoningStyle: core.REASON_REACT, Messages: []core.Message{
		{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "watch log"}}},
	}}
	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	assert.Equal(t, planning.THINK_THEN_ACT_DISPATCH, scratchGet(out, planning.THINK_THEN_ACT_PHASE))
}

func TestThinkThenActDispatchEmitsCallTool(t *testing.T) {
	p := planning.NewThinkThenAct()
	s := core.State{ReasoningStyle: core.REASON_REACT}
	planning.SeedDispatch(&s, core.ToolCall{ID: "c1", Name: "read_log_tail", Args: map[string]any{"n": 5}})

	_, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_TOOL, instrs[0].Kind)
	require.NotNil(t, instrs[0].CallTool)
	assert.Equal(t, "c1", instrs[0].CallTool.Call.ID)
	assert.Equal(t, "read_log_tail", instrs[0].CallTool.Call.Name)
}

func TestThinkThenActReflectEmitsCallModel(t *testing.T) {
	p := planning.NewThinkThenAct()
	s := core.State{ReasoningStyle: core.REASON_REACT, WorkingMemory: map[string]any{
		planning.THINK_THEN_ACT_PHASE: planning.THINK_THEN_ACT_REFLECT,
	}}
	_, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
}

func TestPlanThenRunSkipsToExecuteWhenBlueprintSeeded(t *testing.T) {
	p := planning.NewPlanThenRun()
	s := core.State{ReasoningStyle: core.REASON_PLAN_THEN_RUN}
	planning.SeedBlueprint(&s, []core.ToolCall{
		{ID: "s1", Name: "add_todo", Args: map[string]any{"title": "investigate"}},
		{ID: "s2", Name: "add_todo", Args: map[string]any{"title": "fix"}},
	})

	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_TOOL, instrs[0].Kind)
	assert.Equal(t, "s1", instrs[0].CallTool.Call.ID)
	assert.Equal(t, 1, scratchGetInt(out, planning.PLAN_THEN_RUN_STEP_INDEX))
	assert.Equal(t, planning.RUN_PHASE_PTR, scratchGet(out, planning.PLAN_THEN_RUN_PHASE))
}

func TestPlanThenRunEmitsDoneWhenBlueprintExhausted(t *testing.T) {
	p := planning.NewPlanThenRun()
	s := core.State{ReasoningStyle: core.REASON_PLAN_THEN_RUN, WorkingMemory: map[string]any{
		planning.PLAN_THEN_RUN_PHASE:      planning.RUN_PHASE_PTR,
		planning.PLAN_THEN_RUN_STEP_INDEX: 2,
		planning.PLAN_THEN_RUN_BLUEPRINT: []core.ToolCall{
			{ID: "s1", Name: "a"},
			{ID: "s2", Name: "b"},
		},
	}}
	_, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_DONE, instrs[0].Kind)
}

func TestRunThenReviewPassedEmitsDone(t *testing.T) {
	p := planning.NewRunThenReview()
	s := core.State{ReasoningStyle: core.REASON_DO_THEN_REVIEW}
	planning.SeedReviewPassed(&s, "looks good")

	_, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_DONE, instrs[0].Kind)
}

func TestRunThenReviewFailedEmitsCallModel(t *testing.T) {
	p := planning.NewRunThenReview()
	s := core.State{ReasoningStyle: core.REASON_DO_THEN_REVIEW}
	planning.SeedReviewFailed(&s, "step missing precondition")

	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	assert.Equal(t, planning.RUN_PHASE, scratchGet(out, planning.RUN_THEN_REVIEW_PHASE))
	assert.Equal(t, 1, scratchGetInt(out, planning.RUN_THEN_REVIEW_ITERATION))
}

func TestStubRulesDoNotPanic(t *testing.T) {
	tests := []struct {
		name string
		rule core.DecisionRule
		kind core.ReasoningStyle
	}{
		{"one_shot", planning.NewOneShotReasoning(), core.REASON_ONE_SHOT},
		{"learn_from_failure", planning.NewLearnFromFailure(), core.REASON_LEARN_FROM_FAILURE},
		{"choose_agent", planning.NewChooseAgent(), core.REASON_PICK_AGENT},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.kind, tc.rule.Kind())
			s := core.State{ReasoningStyle: tc.kind}
			out, instrs := tc.rule.NextStep(s)
			assert.NotEmpty(t, instrs, "stub must return at least one instruction")
			// every stub must include a DONE so the run can terminate
			hasDone := false
			for _, ins := range instrs {
				if ins.Kind == core.INSTRUCTION_DONE {
					hasDone = true
				}
			}
			assert.True(t, hasDone, "stub must emit DONE")
			_ = out
		})
	}
}

func scratchGet(s core.State, key string) string {
	if s.WorkingMemory == nil {
		return ""
	}
	v, ok := s.WorkingMemory[key]
	if !ok {
		return ""
	}
	if x, ok := v.(string); ok {
		return x
	}
	return ""
}

func scratchGetInt(s core.State, key string) int {
	if s.WorkingMemory == nil {
		return 0
	}
	v, ok := s.WorkingMemory[key]
	if !ok {
		return 0
	}
	if x, ok := v.(int); ok {
		return x
	}
	return 0
}
