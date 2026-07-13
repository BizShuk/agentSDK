package planning

import "github.com/bizshuk/agentsdk/core"

// Scratch keys for OneShotReasoning — exported so tests / sample can drive the FSM.
const (
	ONE_SHOT_PHASE = "one_shot.phase" // string — "think" (default) | "done"

	// OneShot phase values:
	ONE_SHOT_THINK = "think" // ask LLM to reason once
	ONE_SHOT_DONE  = "done"  // terminal
)

// OneShotReasoning is the one-shot chain-of-thought rule (Wei 2022).
//
// working memory[ONE_SHOT_PHASE] drives a two-phase FSM:
//   think → emit INSTRUCTION_CALL_MODEL, advance to done
//   done  → emit INSTRUCTION_DONE
//
// The phase FSM matters: the old STUB emitted [CALL_MODEL, DONE] on every
// NextStep call, which violates the pure-function invariant (same state must
// yield same result) — a retry / WAL replay would re-issue the model call.
// With the phase, think fires exactly once, then every subsequent call
// returns DONE.
type OneShotReasoning struct{}

// NewOneShotReasoning returns the rule.
func NewOneShotReasoning() *OneShotReasoning { return &OneShotReasoning{} }

// Kind returns REASON_ONE_SHOT.
func (p *OneShotReasoning) Kind() core.ReasoningStyle { return core.REASON_ONE_SHOT }

// NextStep reads working memory and emits the next instruction, advancing
// working memory in place.
//
// Pure: no I/O. The runtime executes the returned instruction, then folds
// the result back into state on the next call.
func (p *OneShotReasoning) NextStep(state core.State) (core.State, []core.Instruction) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, ONE_SHOT_PHASE, ONE_SHOT_THINK)

	switch phase {
	case ONE_SHOT_THINK:
		next := state.Clone()
		scratchSet(&next, ONE_SHOT_PHASE, ONE_SHOT_DONE)
		return next, []core.Instruction{callModelFromMessages(state.Clone())}

	default:
		// ONE_SHOT_DONE or any unknown value — fail-closed to DONE.
		return state, []core.Instruction{doneInstruction()}
	}
}

// SeedOneShotThinking forces the rule into the think phase (default behavior).
// Provided for symmetry with other patterns' Seed* helpers; usually not needed.
func SeedOneShotThinking(s *core.State) {
	scratchSet(s, ONE_SHOT_PHASE, ONE_SHOT_THINK)
}

// SeedOneShotDone forces the rule to emit DONE on the next NextStep.
func SeedOneShotDone(s *core.State) {
	scratchSet(s, ONE_SHOT_PHASE, ONE_SHOT_DONE)
}
