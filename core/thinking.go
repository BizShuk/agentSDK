package core

// ThinkingKind is the selector for which ThinkingPattern owns a step.
type ThinkingKind string

const (
	// THINK_REACT — Reason + Act: think, pick a tool, observe, repeat.
	THINK_REACT ThinkingKind = "react"
	// THINK_PLANNER_EXECUTOR — blueprint first, then execute each step.
	THINK_PLANNER_EXECUTOR ThinkingKind = "planner_executor"
	// THINK_EXECUTOR_CRITIC — execute, then critique; iterate.
	THINK_EXECUTOR_CRITIC ThinkingKind = "executor_critic"
	// THINK_COT_SINGLESHOT — one-shot chain of thought. STUB.
	THINK_COT_SINGLESHOT ThinkingKind = "cot_singleshot"
	// THINK_REFLEXION — remember failures, retry with reflection. STUB.
	THINK_REFLEXION ThinkingKind = "reflexion"
	// THINK_ROUTER — multi-agent router. STUB.
	THINK_ROUTER ThinkingKind = "router"
)

// ThinkingPattern owns one decision rule. Decide is a pure function:
// given the current state, produce (next-state, effects-to-execute).
//
// The actual LLM call / tool invocation is described by the returned effects;
// Think patterns never call I/O directly. This is what lets us replay a run
// from the WAL without re-issuing tool calls.
type ThinkingPattern interface {
	// Kind is the discriminator Step uses to dispatch.
	Kind() ThinkingKind
	// Decide is a pure function — no I/O — and MUST be deterministic given (state, scratch).
	Decide(state State) (State, []Effect)
}
