package planning

import "github.com/bizshuk/agentsdk/core"

const (
	EC_PHASE    = "ec.phase"           // "execute" | "critique" | "iterate" | "done"
	EC_CRITIQUE = "ec.critique_text"   // string — last critique; empty means none
	EC_ITER     = "ec.iteration"       // int

	EC_PHASE_EXEC = "execute"
	EC_PHASE_CRIT = "critique"
	EC_PHASE_DONE = "done"
)

// ExecutorCritic: emit execute → emit critique → if critique flags issues,
// iterate (up to a budget) → else DONE.
type ExecutorCritic struct{}

// NewExecutorCritic returns the pattern.
func NewExecutorCritic() *ExecutorCritic { return &ExecutorCritic{} }

// Kind returns THINK_EXECUTOR_CRITIC.
func (p *ExecutorCritic) Kind() core.ThinkingKind { return core.THINK_EXECUTOR_CRITIC }

func (p *ExecutorCritic) Decide(state core.State) (core.State, []core.Effect) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, EC_PHASE, EC_PHASE_EXEC)

	switch phase {
	case EC_PHASE_EXEC:
		next := state.Clone()
		scratchSet(&next, EC_PHASE, EC_PHASE_CRIT)
		return next, []core.Effect{callModelFromMessages(state.Clone())}

	case EC_PHASE_CRIT:
		crit := scratchString(state, EC_CRITIQUE, "")
		if crit == "" || hasOKPrefix(crit) {
			next := state.Clone()
			scratchSet(&next, EC_PHASE, EC_PHASE_DONE)
			return next, []core.Effect{doneEffect()}
		}
		// Otherwise iterate: bump iteration, return to execute.
		iter := scratchInt(state, EC_ITER, 0)
		next := state.Clone()
		scratchSet(&next, EC_PHASE, EC_PHASE_EXEC)
		scratchSet(&next, EC_ITER, iter+1)
		return next, []core.Effect{callModelFromMessages(state.Clone())}

	default:
		return state, []core.Effect{doneEffect()}
	}
}

// SeedCritiqueOK tells the pattern the previous critique was passing — next Decide emits DONE.
func SeedCritiqueOK(s *core.State, text string) {
	scratchSet(s, EC_PHASE, EC_PHASE_CRIT)
	scratchSet(s, EC_CRITIQUE, "OK: "+text)
}

// SeedCritiqueReject tells the pattern to iterate.
func SeedCritiqueReject(s *core.State, text string) {
	scratchSet(s, EC_PHASE, EC_PHASE_CRIT)
	scratchSet(s, EC_CRITIQUE, text)
}
