package planning

import (
	"fmt"

	"github.com/bizshuk/agentsdk/core"
)

// Scratch keys for ChooseAgent (Router / Orchestrator) — exported so tests /
// sample can drive the FSM.
const (
	CHOOSE_AGENT_PHASE      = "choose_agent.phase"       // string — "select" (default) | "delegate" | "done"
	CHOOSE_AGENT_AGENT_LIST = "choose_agent.agent_list"  // []string — agent names; seed-only
	CHOOSE_AGENT_CHOSEN     = "choose_agent.chosen_agent" // string — the picked agent name

	// ChooseAgent phase values:
	CA_SELECT   = "select"   // pick an agent from the list (or LLM-routing hook)
	CA_DELEGATE = "delegate" // hand the task to the chosen agent
	CA_DONE     = "done"    // terminal
)

// ChooseAgent is the multi-agent router rule.
//
// working memory[CHOOSE_AGENT_PHASE] drives a three-phase FSM:
//   select   → if CHOOSE_AGENT_AGENT_LIST is non-empty, pick [0] into
//             CHOOSE_AGENT_CHOSEN and emit NOTIFY (info, "router chose agent: <name>");
//             otherwise emit INSTRUCTION_CALL_MODEL as a hook for future
//             LLM-driven routing. Either way, advance to delegate.
//   delegate → emit INSTRUCTION_CALL_MODEL with a system-message prefix
//             ("You are agent <chosen>. ...") prepended to state.Messages —
//             the closest in-scope approximation of delegation without a
//             sub-agent registry.
//   done     → emit INSTRUCTION_DONE
//
// No sub-agent registry exists yet (that belongs to the action.ToolSource
// dynamic-registration wave), so ChooseAgent does not actually spawn a
// sub-agent. It records the routing decision in scratch and re-asks the
// model in the chosen agent's voice.
type ChooseAgent struct{}

// NewChooseAgent returns the rule.
func NewChooseAgent() *ChooseAgent { return &ChooseAgent{} }

// Kind returns REASON_PICK_AGENT.
func (p *ChooseAgent) Kind() core.ReasoningStyle { return core.REASON_PICK_AGENT }

func (p *ChooseAgent) NextStep(state core.State) (core.State, []core.Instruction) {
	state.UpdatedAt = nowOrZero(state)
	phase := scratchString(state, CHOOSE_AGENT_PHASE, CA_SELECT)

	switch phase {
	case CA_SELECT:
		next := state.Clone()
		scratchSet(&next, CHOOSE_AGENT_PHASE, CA_DELEGATE)
		if agents := scratchStringSlice(state, CHOOSE_AGENT_AGENT_LIST); len(agents) > 0 {
			chosen := agents[0]
			scratchSet(&next, CHOOSE_AGENT_CHOSEN, chosen)
			return next, []core.Instruction{{
				Kind: core.INSTRUCTION_NOTIFY,
				Notify: &core.NotifyInstruction{
					Level:   "info",
					Message: fmt.Sprintf("router chose agent: %s", chosen),
				},
			}}
		}
		// No agent list seeded — fall back to a CALL_MODEL hook so a future
		// LLM-driven router can populate CHOOSE_AGENT_CHOSEN.
		return next, []core.Instruction{callModelFromMessages(state.Clone())}

	case CA_DELEGATE:
		next := state.Clone()
		scratchSet(&next, CHOOSE_AGENT_PHASE, CA_DONE)
		chosen := scratchString(state, CHOOSE_AGENT_CHOSEN, "")
		// Inline-build the CALL_MODEL: prepend a system message that frames the
		// chosen agent, then the original transcript. This is the one pattern
		// that bypasses callModelFromMessages, because it needs a system-prefix
		// the shared helper does not support.
		msgs := make([]core.Message, 0, len(next.Messages)+1)
		msgs = append(msgs, core.Message{
			Role: core.ROLE_SYSTEM,
			Parts: []core.Part{{
				Kind: core.PART_KIND_PLAIN_TEXT,
				Text: fmt.Sprintf("You are agent %s. Address the user task in that agent's voice.", chosen),
			}},
		})
		msgs = append(msgs, next.Messages...)
		return next, []core.Instruction{{
			Kind: core.INSTRUCTION_CALL_MODEL,
			CallModel: &core.CallModelInstruction{
				RequestID: newID(),
				Messages:  msgs,
			},
		}}

	default:
		// CA_DONE or any unknown value — fail-closed to DONE.
		return state, []core.Instruction{doneInstruction()}
	}
}

// SeedAgents installs a list of agent names. The next NextStep in the select
// phase picks agents[0] into CHOOSE_AGENT_CHOSEN and emits NOTIFY. Without
// this seed the rule falls through to a CALL_MODEL hook for future
// LLM-driven routing.
func SeedAgents(s *core.State, agents []string) {
	scratchSet(s, CHOOSE_AGENT_AGENT_LIST, agents)
}
