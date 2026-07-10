package planning

import "github.com/bizshuk/agentsdk/core"

// LearnFromFailure: remember failures, retry with reflection (Shinn 2023).
//
// STUB: emits a single INSTRUCTION_CALL_MODEL and INSTRUCTION_DONE.
type LearnFromFailure struct{}

// NewLearnFromFailure returns the stub rule.
func NewLearnFromFailure() *LearnFromFailure { return &LearnFromFailure{} }

// Kind returns REASON_LEARN_FROM_FAILURE.
func (p *LearnFromFailure) Kind() core.ReasoningStyle { return core.REASON_LEARN_FROM_FAILURE }

// NextStep emits a single INSTRUCTION_CALL_MODEL and INSTRUCTION_DONE.
func (p *LearnFromFailure) NextStep(state core.State) (core.State, []core.Instruction) {
	next := state.Clone()
	next.UpdatedAt = nowOrZero(state)
	return next, []core.Instruction{
		callModelFromMessages(state.Clone()),
		doneInstruction(),
	}
}
