package core

// Decide is the pure transition function. Given (state, event) it produces
// the next state and a list of instructions the runtime must execute.
//
// Strict contract:
//   - no I/O
//   - deterministic given (state, working memory)
//   - returns a nil State change when nothing changed
//   - returns an empty instruction slice ONLY for terminal events (HumanDecision / Resume);
//
//	otherwise an empty slice means the rule did not act — the runtime
//	will surface this as a stuck loop via loopguard in M2.
//
// NewDecide is the single dispatch point: it picks the DecisionRule by
// state.ReasoningStyle. Callers cannot bypass the registry.
type Decide func(state State, event Event) (State, []Instruction)

// NewDecide returns a Decide that dispatches on state.ReasoningStyle.
//
// Reasoning lives in the per-rule NextStep methods. The runtime only needs
// to feed events in order. The two-stage call (set kind → call NextStep)
// keeps the Decide contract pure: state carries the routing decision.
func NewDecide(rules map[ReasoningStyle]DecisionRule) Decide {
	return func(state State, event Event) (State, []Instruction) {
		kind := state.ReasoningStyle
		r, ok := rules[kind]
		if !ok {
			// Rule not registered — emit a NOTIFY instruction and return unchanged.
			return state, []Instruction{
				{Kind: INSTRUCTION_NOTIFY, Notify: &NotifyInstruction{
					Level:   "error",
					Message: "unknown reasoning style: " + string(kind),
				}},
			}
		}
		return r.NextStep(state)
	}
}
