package reasoning_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/reasoning"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRule is the minimal DecisionRule for testing NewDecide dispatch.
type stubRule struct {
	kind   string
	instrs []core.Instruction
	called int
}

func (s *stubRule) Kind() string { return s.kind }
func (s *stubRule) NextStep(state core.State) (core.State, []core.Instruction) {
	s.called++
	return state, s.instrs
}

func TestNewDecideDispatchesByKind(t *testing.T) {
	react := &stubRule{kind: core.REASON_REACT, instrs: []core.Instruction{
		{Kind: core.INSTRUCTION_CALL_MODEL, CallModel: &core.ModelRequest{RequestID: "r1"}},
	}}
	plans := &stubRule{kind: core.REASON_PLAN_THEN_RUN, instrs: []core.Instruction{
		{Kind: core.INSTRUCTION_DONE},
	}}

	decide := reasoning.NewDecide(map[string]reasoning.DecisionRule{
		core.REASON_REACT:         react,
		core.REASON_PLAN_THEN_RUN: plans,
	})

	t.Run("react path", func(t *testing.T) {
		state := core.State{ReasoningStyle: core.REASON_REACT}
		_, instrs := decide(state, core.Event{Kind: core.EVENT_OBSERVATION})
		assert.Equal(t, 1, react.called)
		assert.Equal(t, 0, plans.called)
		require.Len(t, instrs, 1)
		assert.Equal(t, core.INSTRUCTION_CALL_MODEL, instrs[0].Kind)
	})

	t.Run("planner path", func(t *testing.T) {
		state := core.State{ReasoningStyle: core.REASON_PLAN_THEN_RUN}
		out, instrs := decide(state, core.Event{Kind: core.EVENT_OBSERVATION})
		assert.Equal(t, 1, plans.called)
		require.Len(t, instrs, 1)
		assert.Equal(t, core.INSTRUCTION_DONE, instrs[0].Kind)
		assert.Equal(t, core.REASON_PLAN_THEN_RUN, out.ReasoningStyle)
	})

	t.Run("unknown kind surfaces NOTIFY", func(t *testing.T) {
		state := core.State{ReasoningStyle: "weird"}
		_, instrs := decide(state, core.Event{Kind: core.EVENT_OBSERVATION})
		require.Len(t, instrs, 1)
		assert.Equal(t, core.INSTRUCTION_NOTIFY, instrs[0].Kind)
		require.NotNil(t, instrs[0].Notify)
		assert.Equal(t, "error", instrs[0].Notify.Level)
	})
}
