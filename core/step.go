package core

// Step is the pure transition function. Given (state, input) it produces
// the next state and a list of effects the runtime must execute.
//
// Strict contract:
//   - no I/O
//   - deterministic given (state, scratch)
//   - returns a nil State change when nothing changed
//   - returns an empty effect slice ONLY for terminal inputs (ApprovalDecision / Resume);
//
//	otherwise an empty slice means the pattern did not act — the runtime
//	will surface this as a stuck loop via loopguard in M2.
//
// NewStep is the single dispatch point: it picks the ThinkingPattern by
// state.ThinkingKind. Callers cannot bypass the registry.
type Step func(state State, input Input) (State, []Effect)

// NewStep returns a Step that dispatches on state.ThinkingKind.
//
// Reasoning lives in the per-pattern Decide methods. The runtime only needs
// to feed inputs in order. The two-stage call (set kind → call Decide) keeps
// the Step contract pure: state carries the routing decision.
func NewStep(patterns map[ThinkingKind]ThinkingPattern) Step {
	return func(state State, input Input) (State, []Effect) {
		kind := state.ThinkingKind
		p, ok := patterns[kind]
		if !ok {
			// Pattern not registered — emit a NOTIFY effect and return unchanged.
			return state, []Effect{
				{Kind: EFFECT_NOTIFY, Notify: &NotifyEffect{
					Level:   "error",
					Message: "unknown thinking kind: " + string(kind),
				}},
			}
		}
		return p.Decide(state)
	}
}
