package reasoning

import "github.com/bizshuk/agentsdk/core"

const (
	RUN_THEN_REVIEW_PHASE     = "do_then_review.phase"     // "execute" | "critique" | "done"
	RUN_THEN_REVIEW_NOTE      = "do_then_review.note"      // string — last review; empty means none
	RUN_THEN_REVIEW_ITERATION = "do_then_review.iteration" // int

	RUN_PHASE    = "execute"
	REVIEW_PHASE = "critique"
	DONE_PHASE   = "done"
)

// RunThenReview: emit execute → emit review → if review flags issues,
// iterate (up to a budget) → else INSTRUCTION_DONE.
type RunThenReview struct{}

// NewRunThenReview returns the rule.
func NewRunThenReview() *RunThenReview { return &RunThenReview{} }

// Kind returns REASON_DO_THEN_REVIEW.
func (p *RunThenReview) Kind() string { return core.REASON_DO_THEN_REVIEW }

func (p *RunThenReview) NextStep(state core.State) (core.State, []core.Instruction) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, RUN_THEN_REVIEW_PHASE, RUN_PHASE)

	switch phase {
	case RUN_PHASE:
		next := state.Clone()
		scratchSet(&next, RUN_THEN_REVIEW_PHASE, REVIEW_PHASE)
		return next, []core.Instruction{callModelFromMessages(state.Clone())}

	case REVIEW_PHASE:
		note := scratchString(state, RUN_THEN_REVIEW_NOTE, "")
		if note == "" || startsWithPassed(note) {
			next := state.Clone()
			scratchSet(&next, RUN_THEN_REVIEW_PHASE, DONE_PHASE)
			return next, []core.Instruction{doneInstruction()}
		}
		// Otherwise iterate: bump iteration, return to execute.
		iter := scratchInt(state, RUN_THEN_REVIEW_ITERATION, 0)
		next := state.Clone()
		scratchSet(&next, RUN_THEN_REVIEW_PHASE, RUN_PHASE)
		scratchSet(&next, RUN_THEN_REVIEW_ITERATION, iter+1)
		return next, []core.Instruction{callModelFromMessages(state.Clone())}

	default:
		return state, []core.Instruction{doneInstruction()}
	}
}

// SeedReviewPassed tells the rule the previous review was passing — next NextStep emits DONE.
func SeedReviewPassed(s *core.State, text string) {
	scratchSet(s, RUN_THEN_REVIEW_PHASE, REVIEW_PHASE)
	scratchSet(s, RUN_THEN_REVIEW_NOTE, "OK: "+text)
}

// SeedReviewFailed tells the rule to iterate.
func SeedReviewFailed(s *core.State, text string) {
	scratchSet(s, RUN_THEN_REVIEW_PHASE, REVIEW_PHASE)
	scratchSet(s, RUN_THEN_REVIEW_NOTE, text)
}
