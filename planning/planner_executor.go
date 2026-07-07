package planning

import "github.com/bizshuk/agentsdk/core"

const (
	PLAN_THEN_RUN_BLUEPRINT  = "plan_then_run.blueprint"  // []ToolCall
	PLAN_THEN_RUN_STEP_INDEX = "plan_then_run.step_index" // int
	PLAN_THEN_RUN_PHASE      = "plan_then_run.phase"      // "plan" | "execute" | "done"

	PLAN_PHASE     = "plan"
	RUN_PHASE_PTR  = "execute"
	DONE_PHASE_PTR = "done"
)

// PlanThenRun first asks the LLM for a blueprint (an ordered list of
// tool calls), then dispatches them one at a time.
// working memory[PLAN_THEN_RUN_BLUEPRINT] holds the decoded blueprint;
// once it is non-empty, the rule drives successive INSTRUCTION_CALL_TOOL
// instructions until the list is exhausted.
type PlanThenRun struct{}

// NewPlanThenRun returns the rule.
func NewPlanThenRun() *PlanThenRun { return &PlanThenRun{} }

// Kind returns REASON_PLAN_THEN_RUN.
func (p *PlanThenRun) Kind() core.ReasoningStyle { return core.REASON_PLAN_THEN_RUN }

// NextStep uses working memory[PLAN_THEN_RUN_PHASE]:
//   plan    → emit INSTRUCTION_CALL_MODEL with a "produce a blueprint" prompt
//   execute → emit INSTRUCTION_CALL_TOOL for blueprint[step_index] then bump
//   done    → emit INSTRUCTION_DONE
func (p *PlanThenRun) NextStep(state core.State) (core.State, []core.Instruction) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, PLAN_THEN_RUN_PHASE, PLAN_PHASE)

	switch phase {
	case PLAN_PHASE:
		// If a blueprint was previously installed (e.g. from a model reply),
		// skip planning and move to execute.
		if blueprint, ok := scratchBlueprint(state); ok && len(blueprint) > 0 {
			next := state.Clone()
			scratchSet(&next, PLAN_THEN_RUN_PHASE, RUN_PHASE_PTR)
			scratchSet(&next, PLAN_THEN_RUN_STEP_INDEX, 0)
			return next, []core.Instruction{callToolInstruction(blueprint[0])}
		}
		return state, []core.Instruction{callModelFromMessages(state.Clone())}

	case RUN_PHASE_PTR:
		blueprint, ok := scratchBlueprint(state)
		if !ok || len(blueprint) == 0 {
			next := state.Clone()
			scratchSet(&next, PLAN_THEN_RUN_PHASE, DONE_PHASE_PTR)
			return next, []core.Instruction{doneInstruction()}
		}
		idx := scratchInt(state, PLAN_THEN_RUN_STEP_INDEX, 0)
		if idx >= len(blueprint) {
			next := state.Clone()
			scratchSet(&next, PLAN_THEN_RUN_PHASE, DONE_PHASE_PTR)
			return next, []core.Instruction{doneInstruction()}
		}
		next := state.Clone()
		scratchSet(&next, PLAN_THEN_RUN_STEP_INDEX, idx+1)
		return next, []core.Instruction{callToolInstruction(blueprint[idx])}

	default:
		return state, []core.Instruction{doneInstruction()}
	}
}

// SeedBlueprint installs a blueprint so the next NextStep emits
// INSTRUCTION_CALL_TOOL for step 0.
func SeedBlueprint(s *core.State, blueprint []core.ToolCall) {
	scratchSet(s, PLAN_THEN_RUN_BLUEPRINT, blueprint)
	scratchSet(s, PLAN_THEN_RUN_PHASE, RUN_PHASE_PTR)
	scratchSet(s, PLAN_THEN_RUN_STEP_INDEX, 0)
}
