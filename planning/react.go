// Package planning hosts the six ThinkingPattern implementations.
// Pattern Decide is a pure reducer: read state, write effects + scratch.
// No I/O — runtime does dispatching based on the effects returned.
package planning

import "github.com/bizshuk/agentsdk/core"

// Scratch keys for ReAct — exported so tests / sample can drive the FSM.
const (
	REACT_PHASE       = "react.phase"
	REACT_LAST_CALL   = "react.last_call_id"
	REACT_LAST_RESULT = "react.last_result_signature"

	// ReAct phase values:
	REACT_PHASE_THINK    = "think"    // ask LLM to reason
	REACT_PHASE_ACT      = "act"      // dispatch a tool call
	REACT_PHASE_OBSERVE  = "observe"  // reason over tool result
)

// ReAct implements the classic Reason+Act loop.
//
// scratch[REACT_PHASE] drives the FSM:
//   think    → emit CALL_MODEL
//   act      → emit CALL_TOOL for scratch[REACT_LAST_CALL]
//   observe  → emit CALL_MODEL to reason over the prior result
//
// In M1 ReAct does not parse the model output — scratch is the lingua
// franca between the runtime and the pattern. Sample / fixtures set the
// phase via scratch, runtime's normal loop sets it after each dispatch.
type ReAct struct{}

// NewReAct returns the ReAct pattern.
func NewReAct() *ReAct { return &ReAct{} }

// Kind returns THINK_REACT.
func (p *ReAct) Kind() core.ThinkingKind { return core.THINK_REACT }

// Decide reads scratch and emits the next effect, advancing scratch in place.
//
// Pure: no I/O. The runtime executes the returned effect, then folds the
// result back into state on the next call.
func (p *ReAct) Decide(state core.State) (core.State, []core.Effect) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, REACT_PHASE, REACT_PHASE_THINK)

	switch phase {
	case REACT_PHASE_THINK:
		next := state.Clone()
		scratchSet(&next, REACT_PHASE, REACT_PHASE_ACT)
		return next, []core.Effect{callModelFromMessages(next)}

	case REACT_PHASE_ACT:
		call, ok := scratchCall(state, REACT_LAST_CALL)
		if !ok {
			return state, []core.Effect{doneEffect()}
		}
		next := state.Clone()
		scratchSet(&next, REACT_PHASE, REACT_PHASE_OBSERVE)
		return next, []core.Effect{callToolEffect(call)}

	case REACT_PHASE_OBSERVE:
		next := state.Clone()
		scratchSet(&next, REACT_PHASE, REACT_PHASE_ACT)
		return next, []core.Effect{callModelFromMessages(next)}

	default:
		return state, []core.Effect{doneEffect()}
	}
}

// SeedAct lets a driver (test, fixture) install a pending tool call so
// the next Decide emits CALL_TOOL.
func SeedAct(s *core.State, call core.ToolCall) {
	scratchSet(s, REACT_PHASE, REACT_PHASE_ACT)
	scratchSet(s, REACT_LAST_CALL, call)
}
