package planning

import "github.com/bizshuk/agentsdk/core"

const (
	PE_BLUEPRINT_STEPS = "pe.blueprint"        // []ToolCall
	PE_STEP_INDEX      = "pe.step_index"       // int
	PE_PHASE           = "pe.phase"            // "plan" | "execute" | "done"

	PE_PHASE_PLAN = "plan"
	PE_PHASE_EXEC = "execute"
	PE_PHASE_DONE = "done"
)

// PlannerExecutor first asks the LLM for a blueprint (an ordered list of
// tool calls), then dispatches them one at a time. scratch[PE_BLUEPRINT_STEPS]
// holds the decoded blueprint; once it is non-empty, the pattern drives
// successive CALL_TOOL effects until the list is exhausted.
type PlannerExecutor struct{}

// NewPlannerExecutor returns the pattern.
func NewPlannerExecutor() *PlannerExecutor { return &PlannerExecutor{} }

// Kind returns THINK_PLANNER_EXECUTOR.
func (p *PlannerExecutor) Kind() core.ThinkingKind { return core.THINK_PLANNER_EXECUTOR }

// Decide uses scratch[PE_PHASE]:
//   plan    → emit CALL_MODEL with a "produce a blueprint" prompt
//   execute → emit CALL_TOOL for blueprint[step_index] then bump
//   done    → emit DONE
func (p *PlannerExecutor) Decide(state core.State) (core.State, []core.Effect) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, PE_PHASE, PE_PHASE_PLAN)

	switch phase {
	case PE_PHASE_PLAN:
		// If a blueprint was previously installed (e.g. from a model reply),
		// skip planning and move to execute.
		if blueprint, ok := scratchBlueprint(state); ok && len(blueprint) > 0 {
			next := state.Clone()
			scratchSet(&next, PE_PHASE, PE_PHASE_EXEC)
			scratchSet(&next, PE_STEP_INDEX, 0)
			return next, []core.Effect{callToolEffect(blueprint[0])}
		}
		return state, []core.Effect{callModelFromMessages(state.Clone())}

	case PE_PHASE_EXEC:
		blueprint, ok := scratchBlueprint(state)
		if !ok || len(blueprint) == 0 {
			next := state.Clone()
			scratchSet(&next, PE_PHASE, PE_PHASE_DONE)
			return next, []core.Effect{doneEffect()}
		}
		idx := scratchInt(state, PE_STEP_INDEX, 0)
		if idx >= len(blueprint) {
			next := state.Clone()
			scratchSet(&next, PE_PHASE, PE_PHASE_DONE)
			return next, []core.Effect{doneEffect()}
		}
		next := state.Clone()
		scratchSet(&next, PE_STEP_INDEX, idx+1)
		return next, []core.Effect{callToolEffect(blueprint[idx])}

	default:
		return state, []core.Effect{doneEffect()}
	}
}

// SeedBlueprint installs a blueprint so the next Decide emits CALL_TOOL for step 0.
func SeedBlueprint(s *core.State, blueprint []core.ToolCall) {
	scratchSet(s, PE_BLUEPRINT_STEPS, blueprint)
	scratchSet(s, PE_PHASE, PE_PHASE_EXEC)
	scratchSet(s, PE_STEP_INDEX, 0)
}
