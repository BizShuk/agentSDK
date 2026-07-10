package planning

import "github.com/bizshuk/agentsdk/core"

// OneShotReasoning is the one-shot chain-of-thought rule (Wei 2022).
//
// STUB: emits exactly one INSTRUCTION_CALL_MODEL and INSTRUCTION_DONE.
type OneShotReasoning struct{}

// NewOneShotReasoning returns the stub rule.
func NewOneShotReasoning() *OneShotReasoning { return &OneShotReasoning{} }

// Kind returns REASON_ONE_SHOT.
func (p *OneShotReasoning) Kind() core.ReasoningStyle { return core.REASON_ONE_SHOT }

// NextStep emits a single INSTRUCTION_CALL_MODEL followed by INSTRUCTION_DONE.
func (p *OneShotReasoning) NextStep(state core.State) (core.State, []core.Instruction) {
	next := state.Clone()
	next.UpdatedAt = nowOrZero(state)
	return next, []core.Instruction{
		callModelFromMessages(state.Clone()),
		doneInstruction(),
	}
}
