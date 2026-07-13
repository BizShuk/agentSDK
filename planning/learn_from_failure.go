package planning

import (
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// Scratch keys for LearnFromFailure (Reflexion, Shinn 2023) — exported so
// tests / sample can drive the FSM.
const (
	LEARN_FROM_FAILURE_PHASE       = "learn_from_failure.phase"       // string — "act" (default) | "reflect" | "retry" | "done"
	LEARN_FROM_FAILURE_REFLECTIONS = "learn_from_failure.reflections" // []string — append-only reflections
	LEARN_FROM_FAILURE_ITERATION   = "learn_from_failure.iteration"   // int — iteration count

	// LearnFromFailure phase values:
	LFF_ACT     = "act"     // first attempt
	LFF_REFLECT = "reflect" // ask LLM to critique the attempt
	LFF_RETRY   = "retry"   // decide pass / fail / retry from the critique
	LFF_DONE    = "done"   // terminal
)

// LearnFromFailure remembers failures and retries with reflection (Shinn 2023).
//
// working memory[LEARN_FROM_FAILURE_PHASE] drives a four-phase FSM:
//   act     → emit INSTRUCTION_CALL_MODEL (first attempt), advance to reflect
//   reflect → emit INSTRUCTION_CALL_MODEL (ask LLM to critique), advance to retry
//   retry   → read latestAssistantText(state): if critique starts with "OK:"
//            emit DONE; otherwise append the critique to LFF_REFLECTIONS,
//            bump iteration, emit INSTRUCTION_CALL_MODEL (retry with accumulated
//            reflection) and stay in retry; if no critique text, DONE (conservative)
//   done    → emit INSTRUCTION_DONE
//
// Failure is judged by the LLM critique's "OK:" prefix (same predicate as
// DoThenReview's startsWithPassed) — not by ToolResult.OK. This keeps the
// rule purely state-driven and requires no runtime preStep seed beyond what
// ReAct already uses. The difference from DoThenReview: LearnFromFailure
// accumulates reflections across iterations into LFF_REFLECTIONS (verbal
// reinforcement), whereas DoThenReview only carries the latest note.
//
// iteration hard cap is enforced by state.Budget (M2 middleware), not by
// the rule — same contract as DoThenReview.
type LearnFromFailure struct{}

// NewLearnFromFailure returns the rule.
func NewLearnFromFailure() *LearnFromFailure { return &LearnFromFailure{} }

// Kind returns REASON_LEARN_FROM_FAILURE.
func (p *LearnFromFailure) Kind() core.ReasoningStyle { return core.REASON_LEARN_FROM_FAILURE }

func (p *LearnFromFailure) NextStep(state core.State) (core.State, []core.Instruction) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, LEARN_FROM_FAILURE_PHASE, LFF_ACT)

	switch phase {
	case LFF_ACT:
		next := state.Clone()
		scratchSet(&next, LEARN_FROM_FAILURE_PHASE, LFF_REFLECT)
		return next, []core.Instruction{callModelFromMessages(state.Clone())}

	case LFF_REFLECT:
		next := state.Clone()
		scratchSet(&next, LEARN_FROM_FAILURE_PHASE, LFF_RETRY)
		return next, []core.Instruction{callModelFromMessages(state.Clone())}

	case LFF_RETRY:
		critique := latestAssistantText(state)
		if critique == "" {
			// No critique to judge — conservatively finish.
			next := state.Clone()
			scratchSet(&next, LEARN_FROM_FAILURE_PHASE, LFF_DONE)
			return next, []core.Instruction{doneInstruction()}
		}
		if startsWithPassed(critique) {
			// Critique approves the attempt — done.
			next := state.Clone()
			scratchSet(&next, LEARN_FROM_FAILURE_PHASE, LFF_DONE)
			return next, []core.Instruction{doneInstruction()}
		}
		// Critique flags a failure — record the reflection and retry.
		next := state.Clone()
		scratchAppendString(&next, LEARN_FROM_FAILURE_REFLECTIONS, critique)
		iter := scratchInt(state, LEARN_FROM_FAILURE_ITERATION, 0)
		scratchSet(&next, LEARN_FROM_FAILURE_ITERATION, iter+1)
		// Stay in retry so the next critique (after this retry's CALL_MODEL)
		// is judged the same way.
		return next, []core.Instruction{callModelFromMessages(state.Clone())}

	default:
		// LFF_DONE or any unknown value — fail-closed to DONE.
		return state, []core.Instruction{doneInstruction()}
	}
}

// SeedLFFAct installs a clean "act" phase (default; usually unnecessary).
func SeedLFFAct(s *core.State) {
	scratchSet(s, LEARN_FROM_FAILURE_PHASE, LFF_ACT)
}

// SeedLFFReflection appends a reflection entry. Tests use this to verify
// accumulation across iterations without driving a real LLM round-trip.
func SeedLFFReflection(s *core.State, text string) {
	scratchAppendString(s, LEARN_FROM_FAILURE_REFLECTIONS, text)
}

// SeedLFFCritiquePassed puts the rule in the retry phase with a passing
// critique ("OK:" prefix) as the latest assistant message, so the next
// NextStep emits DONE. The critique text is appended to state.Messages as
// an assistant plain-text message — mirroring what the runtime would fold
// after the reflect phase's CALL_MODEL returns.
func SeedLFFCritiquePassed(s *core.State, text string) {
	scratchSet(s, LEARN_FROM_FAILURE_PHASE, LFF_RETRY)
	appendAssistantText(s, "OK: "+text)
}

// SeedLFFCritiqueFailed puts the rule in the retry phase with a failing
// critique (no "OK:" prefix) as the latest assistant message, so the next
// NextStep appends the reflection, bumps iteration, and emits CALL_MODEL.
func SeedLFFCritiqueFailed(s *core.State, text string) {
	scratchSet(s, LEARN_FROM_FAILURE_PHASE, LFF_RETRY)
	appendAssistantText(s, text)
}

// appendAssistantText appends an assistant plain-text message to state.Messages.
// Used by the critique seeders to simulate the model's critique reply landing
// in the transcript.
func appendAssistantText(s *core.State, text string) {
	s.Messages = append(s.Messages, core.Message{
		Role:  core.ROLE_ASSISTANT,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}},
		Ts:    time.Now().UTC(),
	})
}
