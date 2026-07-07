package planning

import (
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// nowOrZero returns state.UpdatedAt if zero, else time.Now. Rules use
// this to stamp UpdatedAt consistently without bringing in a clock injection
// at every call site (Test providers inject a clock via Budget.NowFunc in M2).
func nowOrZero(state core.State) time.Time {
	if state.UpdatedAt.IsZero() {
		return time.Now().UTC()
	}
	return state.UpdatedAt
}

func scratchString(state core.State, key, def string) string {
	if state.WorkingMemory == nil {
		return def
	}
	v, ok := state.WorkingMemory[key]
	if !ok {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func scratchInt(state core.State, key string, def int) int {
	if state.WorkingMemory == nil {
		return def
	}
	v, ok := state.WorkingMemory[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return def
}

func scratchCall(state core.State, key string) (core.ToolCall, bool) {
	if state.WorkingMemory == nil {
		return core.ToolCall{}, false
	}
	v, ok := state.WorkingMemory[key]
	if !ok {
		return core.ToolCall{}, false
	}
	tc, ok := v.(core.ToolCall)
	return tc, ok
}

func scratchBlueprint(state core.State) ([]core.ToolCall, bool) {
	if state.WorkingMemory == nil {
		return nil, false
	}
	v, ok := state.WorkingMemory[PLAN_THEN_RUN_BLUEPRINT]
	if !ok {
		return nil, false
	}
	arr, ok := v.([]core.ToolCall)
	return arr, ok
}

func scratchSet(s *core.State, key string, val any) {
	if s.WorkingMemory == nil {
		s.WorkingMemory = make(map[string]any, 4)
	}
	s.WorkingMemory[key] = val
}

func callModelFromMessages(state core.State) core.Instruction {
	return core.Instruction{
		Kind: core.INSTRUCTION_CALL_MODEL,
		CallModel: &core.CallModelInstruction{
			RequestID: newID(),
			Messages:  state.Messages,
		},
	}
}

func callToolInstruction(call core.ToolCall) core.Instruction {
	return core.Instruction{
		Kind:    core.INSTRUCTION_CALL_TOOL,
		CallTool: &core.CallToolInstruction{Call: call},
	}
}

func doneInstruction() core.Instruction {
	return core.Instruction{Kind: core.INSTRUCTION_DONE}
}

// startsWithPassed is the cheap "did the review approve?" predicate.
// Override by populating working memory with a different prefix if needed.
func startsWithPassed(s string) bool {
	if len(s) < 3 {
		return false
	}
	return s[:3] == "OK:"
}

// newID is a process-local id generator. Deterministic tests can replace
// this by setting working memory explicitly; id only appears on emitted instructions.
var idCounter uint64

func newID() string {
	idCounter++
	return formatUint(idCounter)
}

func formatUint(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
