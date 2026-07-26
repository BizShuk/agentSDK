package reasoning

import "github.com/bizshuk/agentsdk/core"

// DecisionRule owns one reduce step. NextStep is a pure function: given
// the current state, produce the next state and instructions to execute.
//
// The actual model call or tool invocation is described by the returned
// instructions; rules never call I/O directly. This lets the runtime replay
// a run from the WAL without reissuing tool calls.
type DecisionRule interface {
	// Kind is the discriminator NewDecide uses to dispatch.
	Kind() string
	// NextStep is deterministic given state and working memory.
	NextStep(state core.State) (core.State, []core.Instruction)
}

// NewDecide returns a pure transition function that dispatches on
// state.ReasoningStyle.
//
// Reasoning lives in each rule's NextStep method. The runtime only feeds
// events in order. The two-stage call (select kind, then call NextStep)
// keeps the transition pure because state carries the routing decision.
func NewDecide(rules map[string]DecisionRule) core.Decide {
	return func(state core.State, _ core.Event) (core.State, []core.Instruction) {
		kind := state.ReasoningStyle
		rule, ok := rules[kind]
		if !ok {
			return state, []core.Instruction{{
				Kind: core.INSTRUCTION_NOTIFY,
				Notify: &core.NotifyInstruction{
					Level:   "error",
					Message: "unknown reasoning style: " + kind,
				},
			}}
		}
		return rule.NextStep(state)
	}
}
