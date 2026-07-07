package planning

import "github.com/bizshuk/agentsdk/core"

// ChooseAgent: multi-agent router rule.
//
// STUB: returns INSTRUCTION_DONE with a notification.
type ChooseAgent struct{}

// NewChooseAgent returns the stub rule.
func NewChooseAgent() *ChooseAgent { return &ChooseAgent{} }

// Kind returns REASON_PICK_AGENT.
func (p *ChooseAgent) Kind() core.ReasoningStyle { return core.REASON_PICK_AGENT }

// NextStep emits INSTRUCTION_DONE with a NOTIFY explaining the stub state.
func (p *ChooseAgent) NextStep(state core.State) (core.State, []core.Instruction) {
	next := state.Clone()
	next.UpdatedAt = nowOrZero(state)
	return next, []core.Instruction{
		{Kind: core.INSTRUCTION_NOTIFY, Notify: &core.NotifyInstruction{
			Level:   "warn",
			Message: "choose_agent rule is a STUB; emitting DONE",
		}},
		doneInstruction(),
	}
}
