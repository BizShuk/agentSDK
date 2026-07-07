// Package planning hosts the six DecisionRule implementations.
// Each rule's NextStep is a pure reducer: read state, write instructions + working memory.
// No I/O — runtime does dispatching based on the instructions returned.
package planning

import "github.com/bizshuk/agentsdk/core"

// Scratch keys for ThinkThenAct — exported so tests / sample can drive the FSM.
const (
	THINK_THEN_ACT_PHASE        = "think_then_act.phase"
	THINK_THEN_ACT_PENDING_CALL = "think_then_act.pending_call"
	THINK_THEN_ACT_LAST_RESULT  = "think_then_act.last_result"

	// ThinkThenAct phase values:
	THINK_THEN_ACT_REASON   = "reason"   // ask LLM to reason
	THINK_THEN_ACT_DISPATCH = "dispatch" // dispatch a tool call
	THINK_THEN_ACT_REFLECT  = "reflect"  // reason over tool result
)

// ThinkThenAct implements the classic Reason+Act loop (Yao 2023).
//
// working memory[THINK_THEN_ACT_PHASE] drives the FSM:
//   reason   → emit INSTRUCTION_CALL_MODEL
//   dispatch → emit INSTRUCTION_CALL_TOOL for working memory[THINK_THEN_ACT_PENDING_CALL]
//   reflect  → emit INSTRUCTION_CALL_MODEL to reason over the prior result
//
// In M1 ThinkThenAct does not parse the model output — working memory is
// the lingua franca between the runtime and the rule. Sample / fixtures
// set the phase via working memory, runtime's normal loop sets it after
// each dispatch.
type ThinkThenAct struct{}

// NewThinkThenAct returns the rule.
func NewThinkThenAct() *ThinkThenAct { return &ThinkThenAct{} }

// Kind returns REASON_REACT.
func (p *ThinkThenAct) Kind() core.ReasoningStyle { return core.REASON_REACT }

// NextStep reads working memory and emits the next instruction, advancing
// working memory in place.
//
// Pure: no I/O. The runtime executes the returned instruction, then folds
// the result back into state on the next call.
func (p *ThinkThenAct) NextStep(state core.State) (core.State, []core.Instruction) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_REASON)

	switch phase {
	case THINK_THEN_ACT_REASON:
		next := state.Clone()
		scratchSet(&next, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_DISPATCH)
		return next, []core.Instruction{callModelFromMessages(next)}

	case THINK_THEN_ACT_DISPATCH:
		call, ok := scratchCall(state, THINK_THEN_ACT_PENDING_CALL)
		if !ok {
			return state, []core.Instruction{doneInstruction()}
		}
		next := state.Clone()
		scratchSet(&next, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_REFLECT)
		return next, []core.Instruction{callToolInstruction(call)}

	case THINK_THEN_ACT_REFLECT:
		next := state.Clone()
		scratchSet(&next, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_DISPATCH)
		return next, []core.Instruction{callModelFromMessages(next)}

	default:
		return state, []core.Instruction{doneInstruction()}
	}
}

// SeedDispatch lets a driver (test, fixture) install a pending tool call so
// the next NextStep emits INSTRUCTION_CALL_TOOL.
func SeedDispatch(s *core.State, call core.ToolCall) {
	scratchSet(s, THINK_THEN_ACT_PHASE, THINK_THEN_ACT_DISPATCH)
	scratchSet(s, THINK_THEN_ACT_PENDING_CALL, call)
}
