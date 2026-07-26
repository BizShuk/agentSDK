package core

const (
	// REASON_REACT — Reason + Act: think, pick a tool, observe, repeat.
	// Yao et al. 2023, "ReAct: Synergizing Reasoning and Acting in Language Models".
	REASON_REACT = "think_then_act"
	// REASON_PLAN_THEN_RUN — blueprint first, then execute each step.
	// Wei et al. 2022, "Chain of Thought Prompting"; planner-executor dispatch.
	REASON_PLAN_THEN_RUN = "plan_then_run"
	// REASON_DO_THEN_REVIEW — execute, then critique; iterate.
	// Welleck et al. 2023, "Self-Refine: Iterative Refinement with Self-Feedback".
	REASON_DO_THEN_REVIEW = "do_then_review"
	// REASON_ONE_SHOT — one-shot chain of thought: reason once, then done.
	// Wei et al. 2022, "Chain of Thought Prompting".
	REASON_ONE_SHOT = "one_shot"
	// REASON_LEARN_FROM_FAILURE — remember failures, retry with reflection.
	// Shinn et al. 2023, "Reflexion: Language Agents with Verbal Reinforcement Learning".
	REASON_LEARN_FROM_FAILURE = "learn_from_failure"
	// REASON_PICK_AGENT — multi-agent router: pick a specialist, delegate.
	REASON_PICK_AGENT = "choose_agent"
)

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
// The reasoning package provides the registry-backed implementation.
type Decide func(state State, event Event) (State, []Instruction)
