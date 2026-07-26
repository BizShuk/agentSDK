package core

// ReasoningStyle is the selector for which DecisionRule owns a step.
type ReasoningStyle string

const (
	// REASON_REACT — Reason + Act: think, pick a tool, observe, repeat.
	// Yao et al. 2023, "ReAct: Synergizing Reasoning and Acting in Language Models".
	REASON_REACT ReasoningStyle = "think_then_act"
	// REASON_PLAN_THEN_RUN — blueprint first, then execute each step.
	// Wei et al. 2022, "Chain of Thought Prompting"; planner-executor dispatch.
	REASON_PLAN_THEN_RUN ReasoningStyle = "plan_then_run"
	// REASON_DO_THEN_REVIEW — execute, then critique; iterate.
	// Welleck et al. 2023, "Self-Refine: Iterative Refinement with Self-Feedback".
	REASON_DO_THEN_REVIEW ReasoningStyle = "do_then_review"
	// REASON_ONE_SHOT — one-shot chain of thought: reason once, then done.
	// Wei et al. 2022, "Chain of Thought Prompting".
	REASON_ONE_SHOT ReasoningStyle = "one_shot"
	// REASON_LEARN_FROM_FAILURE — remember failures, retry with reflection.
	// Shinn et al. 2023, "Reflexion: Language Agents with Verbal Reinforcement Learning".
	REASON_LEARN_FROM_FAILURE ReasoningStyle = "learn_from_failure"
	// REASON_PICK_AGENT — multi-agent router: pick a specialist, delegate.
	REASON_PICK_AGENT ReasoningStyle = "choose_agent"
)

// DecisionRule owns one reduce step. NextStep is a pure function: given
// the current state, produce (next-state, instructions-to-execute).
//
// The actual LLM call / tool invocation is described by the returned
// instructions; rules never call I/O directly. This is what lets us
// replay a run from the WAL without re-issuing tool calls.
type DecisionRule interface {
	// Kind is the discriminator NewDecide uses to dispatch.
	Kind() ReasoningStyle
	// NextStep is a pure function — no I/O — and MUST be deterministic given (state, working memory).
	NextStep(state State) (State, []Instruction)
}

// Decide is the pure transition function. Given (state, event) it produces
// the next state and a list of instructions the runtime must execute.
//
// Strict contract:
//
//   - no I/O
//
//   - deterministic given (state, working memory)
//
//   - returns a nil State change when nothing changed
//
//   - returns an empty instruction slice ONLY for terminal events (HumanDecision / Resume);
//
//     otherwise an empty slice means the rule did not act — the runtime
//     will surface this as a stuck loop via loopguard in M2.
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
