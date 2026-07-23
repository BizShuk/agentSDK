package planning_test

import (
	"encoding/json"
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

// TestRulesReachDone verifies the three formerly-STUB rules (OneShot,
// LearnFromFailure, ChooseAgent) can each reach a terminal DONE state when
// driven by their Seed* helpers. Replaces the old TestStubRulesDoNotPanic,
// which asserted "stub must emit DONE on the first NextStep" — no longer
// valid now that these are real phase FSMs whose first step emits CALL_MODEL.
//
// Each rule is driven with a small loop bounded by maxSteps so a broken FSM
// fails loudly instead of hanging the suite.
func TestRulesReachDone(t *testing.T) {
	tests := []struct {
		name string
		seed func(s *core.State)
		rule core.DecisionRule
	}{
		{
			name: "one_shot",
			seed: func(s *core.State) { /* default phase = think */ },
			rule: planning.NewOneShotReasoning(),
		},
		{
			name: "learn_from_failure",
			seed: func(s *core.State) {
				// Drive act → reflect → retry, then a passing critique ends it.
				planning.SeedLFFCritiquePassed(s, "looks good")
			},
			rule: planning.NewLearnFromFailure(),
		},
		{
			name: "choose_agent",
			seed: func(s *core.State) {
				planning.SeedAgents(s, []string{"router-agent"})
			},
			rule: planning.NewChooseAgent(),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := core.State{ReasoningStyle: tc.rule.Kind()}
			tc.seed(&s)

			const MAX_STEPS = 10
			reachedDone := false
			for i := 0; i < MAX_STEPS; i++ {
				out, instrs := tc.rule.NextStep(s)
				for _, ins := range instrs {
					if ins.Kind == core.INSTRUCTION_DONE {
						reachedDone = true
					}
				}
				if reachedDone {
					break
				}
				s = out
			}
			assert.Truef(t, reachedDone, "rule %s never reached DONE within %d steps", tc.name, MAX_STEPS)
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

// scratchGetStringSlice reads a []string from working memory for assertions.
func scratchGetStringSlice(s core.State, key string) []string {
	if s.WorkingMemory == nil {
		return nil
	}
	v, ok := s.WorkingMemory[key]
	if !ok {
		return nil
	}
	x, ok := v.([]string)
	if !ok {
		return nil
	}
	return x
}

// --- OneShotReasoning ---

func TestOneShotThinkEmitsCallModel(t *testing.T) {
	p := planning.NewOneShotReasoning()
	s := core.State{ReasoningStyle: core.REASON_ONE_SHOT}
	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	assert.Equal(t, planning.ONE_SHOT_DONE, scratchGet(out, planning.ONE_SHOT_PHASE))
}

func TestOneShotDoneEmitsDone(t *testing.T) {
	p := planning.NewOneShotReasoning()
	s := core.State{ReasoningStyle: core.REASON_ONE_SHOT}
	planning.SeedOneShotDone(&s)
	_, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_DONE, instrs[0].Kind)
}

func TestOneShotUnknownPhaseEmitsDone(t *testing.T) {
	p := planning.NewOneShotReasoning()
	s := core.State{
		ReasoningStyle: core.REASON_ONE_SHOT,
		WorkingMemory:  map[string]any{planning.ONE_SHOT_PHASE: "garbage"},
	}
	_, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_DONE, instrs[0].Kind)
}

// --- LearnFromFailure ---

func TestLFFActEmitsCallModel(t *testing.T) {
	p := planning.NewLearnFromFailure()
	s := core.State{ReasoningStyle: core.REASON_LEARN_FROM_FAILURE}
	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	assert.Equal(t, planning.LFF_REFLECT, scratchGet(out, planning.LEARN_FROM_FAILURE_PHASE))
}

func TestLFFReflectEmitsCallModel(t *testing.T) {
	p := planning.NewLearnFromFailure()
	s := core.State{
		ReasoningStyle: core.REASON_LEARN_FROM_FAILURE,
		WorkingMemory:  map[string]any{planning.LEARN_FROM_FAILURE_PHASE: planning.LFF_REFLECT},
	}
	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	assert.Equal(t, planning.LFF_RETRY, scratchGet(out, planning.LEARN_FROM_FAILURE_PHASE))
}

func TestLFFRetryPassedEmitsDone(t *testing.T) {
	p := planning.NewLearnFromFailure()
	s := core.State{ReasoningStyle: core.REASON_LEARN_FROM_FAILURE}
	planning.SeedLFFCritiquePassed(&s, "looks good")
	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_DONE, instrs[0].Kind)
	assert.Equal(t, planning.LFF_DONE, scratchGet(out, planning.LEARN_FROM_FAILURE_PHASE))
}

func TestLFFRetryFailedEmitsCallModel(t *testing.T) {
	p := planning.NewLearnFromFailure()
	s := core.State{ReasoningStyle: core.REASON_LEARN_FROM_FAILURE}
	planning.SeedLFFCritiqueFailed(&s, "step missing precondition")
	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	assert.Equal(t, 1, scratchGetInt(out, planning.LEARN_FROM_FAILURE_ITERATION))
	// Reflection appended from the critique text.
	reflections := scratchGetStringSlice(out, planning.LEARN_FROM_FAILURE_REFLECTIONS)
	assert.Contains(t, reflections, "step missing precondition")
}

func TestLFFReflectionAccumulates(t *testing.T) {
	s := core.State{
		ReasoningStyle: core.REASON_LEARN_FROM_FAILURE,
		WorkingMemory: map[string]any{
			planning.LEARN_FROM_FAILURE_REFLECTIONS: []string{"old"},
		},
	}
	planning.SeedLFFReflection(&s, "new")
	got := scratchGetStringSlice(s, planning.LEARN_FROM_FAILURE_REFLECTIONS)
	assert.Equal(t, []string{"old", "new"}, got)
}

// --- ChooseAgent ---

func TestChooseAgentSelectWithListEmitsNotify(t *testing.T) {
	p := planning.NewChooseAgent()
	s := core.State{ReasoningStyle: core.REASON_PICK_AGENT}
	planning.SeedAgents(&s, []string{"a", "b"})
	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_NOTIFY, instrs[0].Kind)
	require.NotNil(t, instrs[0].Notify)
	assert.Equal(t, "info", instrs[0].Notify.Level)
	assert.Contains(t, instrs[0].Notify.Message, "a")
	assert.Equal(t, "a", scratchGet(out, planning.CHOOSE_AGENT_CHOSEN))
	assert.Equal(t, planning.CA_DELEGATE, scratchGet(out, planning.CHOOSE_AGENT_PHASE))
}

func TestChooseAgentSelectEmptyEmitsCallModel(t *testing.T) {
	p := planning.NewChooseAgent()
	s := core.State{ReasoningStyle: core.REASON_PICK_AGENT}
	out, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	assert.Equal(t, planning.CA_DELEGATE, scratchGet(out, planning.CHOOSE_AGENT_PHASE))
}

func TestChooseAgentDelegateEmitsCallModelWithSystemMsg(t *testing.T) {
	p := planning.NewChooseAgent()
	s := core.State{
		ReasoningStyle: core.REASON_PICK_AGENT,
		WorkingMemory: map[string]any{
			planning.CHOOSE_AGENT_PHASE:  planning.CA_DELEGATE,
			planning.CHOOSE_AGENT_CHOSEN: "a",
		},
		Messages: []core.Message{{
			Role: core.ROLE_USER,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "do the thing"}},
		}},
	}
	_, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	require.NotNil(t, instrs[0].CallModel)
	msgs := instrs[0].CallModel.Messages
	require.NotEmpty(t, msgs)
	assert.Equal(t, core.ROLE_SYSTEM, msgs[0].Role)
	require.NotEmpty(t, msgs[0].Parts)
	assert.Contains(t, msgs[0].Parts[0].Text, "agent a")
}

func TestChooseAgentDoneEmitsDone(t *testing.T) {
	p := planning.NewChooseAgent()
	s := core.State{
		ReasoningStyle: core.REASON_PICK_AGENT,
		WorkingMemory:  map[string]any{planning.CHOOSE_AGENT_PHASE: planning.CA_DONE},
	}
	_, instrs := p.NextStep(s)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_DONE, instrs[0].Kind)
}

// --- helpers ---

func TestLatestAssistantTextFindsLast(t *testing.T) {
	s := core.State{Messages: []core.Message{
		{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "u"}}},
		{Role: core.ROLE_ASSISTANT, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "a1"}}},
		{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "u2"}}},
		{Role: core.ROLE_ASSISTANT, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "a2"}}},
	}}
	// latestAssistantText is unexported; assert indirectly by driving LFF retry,
	// which reads it. Seed a critique and confirm the rule sees "a2".
	p := planning.NewLearnFromFailure()
	// Build state in retry phase with the messages above.
	st := core.State{
		ReasoningStyle: core.REASON_LEARN_FROM_FAILURE,
		WorkingMemory:  map[string]any{planning.LEARN_FROM_FAILURE_PHASE: planning.LFF_RETRY},
		Messages:       s.Messages,
	}
	// "a2" does not start with "OK:" → retry branch: emits CALL_MODEL, appends reflection.
	out, instrs := p.NextStep(st)
	assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	assert.Contains(t, scratchGetStringSlice(out, planning.LEARN_FROM_FAILURE_REFLECTIONS), "a2")
}

func TestLatestAssistantTextEmpty(t *testing.T) {
	p := planning.NewLearnFromFailure()
	// retry phase with no assistant message → conservative DONE.
	st := core.State{
		ReasoningStyle: core.REASON_LEARN_FROM_FAILURE,
		WorkingMemory:  map[string]any{planning.LEARN_FROM_FAILURE_PHASE: planning.LFF_RETRY},
		Messages: []core.Message{{
			Role: core.ROLE_USER,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "u"}},
		}},
	}
	_, instrs := p.NextStep(st)
	require.Len(t, instrs, 1)
	assert.Equal(t, core.INSTRUCTION_DONE, instrs[0].Kind)
}

// TestDispatchSurvivesJSONRoundTrip pins the crash-recovery path.
//
// StateStore persists State as JSON, so a working-memory value written
// in-process as core.ToolCall reads back as map[string]any. The dispatch
// phase used to type-assert, get false, and emit DONE — a run that
// crashed mid-dispatch silently completed on Resume instead of
// re-issuing the call. Decoding by shape is what makes Resume correct.
func TestDispatchSurvivesJSONRoundTrip(t *testing.T) {
	seed := core.State{ReasoningStyle: core.REASON_REACT}
	planning.SeedDispatch(&seed,
		core.ToolCall{ID: "c1", Name: "read", Args: map[string]any{"path": "a.txt"}},
		core.ToolCall{ID: "c2", Name: "read", Args: map[string]any{"path": "b.txt"}},
	)

	raw, err := json.Marshal(seed)
	require.NoError(t, err)
	var reloaded core.State
	require.NoError(t, json.Unmarshal(raw, &reloaded))

	_, insts := planning.NewThinkThenAct().NextStep(reloaded)
	require.Len(t, insts, 2, "both calls must survive the round trip")
	for i, want := range []string{"c1", "c2"} {
		assert.Equal(t, core.INSTRUCTION_CALL_TOOL, insts[i].Kind)
		require.NotNil(t, insts[i].CallTool)
		assert.Equal(t, want, insts[i].CallTool.Call.ID)
	}
}

// TestDispatchReadsLegacySingularKey covers state written before batch
// dispatch existed: one ToolCall under the singular key must still run.
func TestDispatchReadsLegacySingularKey(t *testing.T) {
	s := core.State{
		ReasoningStyle: core.REASON_REACT,
		WorkingMemory: map[string]any{
			planning.THINK_THEN_ACT_PHASE:        planning.THINK_THEN_ACT_DISPATCH,
			planning.THINK_THEN_ACT_PENDING_CALL: core.ToolCall{ID: "old", Name: "read"},
		},
	}
	_, insts := planning.NewThinkThenAct().NextStep(s)
	require.Len(t, insts, 1)
	require.NotNil(t, insts[0].CallTool)
	assert.Equal(t, "old", insts[0].CallTool.Call.ID)
}
