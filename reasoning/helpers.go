package reasoning

import (
	"encoding/json"
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

// scratchCalls reads a pending tool-call batch from working memory.
func scratchCalls(state core.State, key string) []core.ToolCall {
	if state.WorkingMemory == nil {
		return nil
	}
	return decodeCalls(state.WorkingMemory[key])
}

// decodeCalls normalizes every shape a pending-call entry can take.
//
// Working memory survives a JSON round-trip through StateStore, so a
// value written in-process as core.ToolCall reads back after a Load as
// map[string]any. The plain type assertion this replaces returned false
// there, and ThinkThenAct's dispatch phase then emitted DONE instead of
// re-issuing the call — a crash mid-dispatch silently completed the run
// instead of resuming it. Decoding by shape removes that whole class of
// bug rather than patching the one path that surfaced it.
//
// The singular cases are kept because state persisted before batch
// dispatch stores a single ToolCall.
func decodeCalls(v any) []core.ToolCall {
	switch x := v.(type) {
	case nil:
		return nil
	case []core.ToolCall:
		return x
	case core.ToolCall:
		return []core.ToolCall{x}
	default:
		raw, err := json.Marshal(x)
		if err != nil {
			return nil
		}
		var many []core.ToolCall
		if err := json.Unmarshal(raw, &many); err == nil && len(many) > 0 {
			return many
		}
		var one core.ToolCall
		if err := json.Unmarshal(raw, &one); err == nil && one.Name != "" {
			return []core.ToolCall{one}
		}
		return nil
	}
}

// scratchStringSlice reads a []string from working memory. Missing key or
// wrong type returns nil. Used by LearnFromFailure for the append-only
// reflections slice.
func scratchStringSlice(state core.State, key string) []string {
	if state.WorkingMemory == nil {
		return nil
	}
	v, ok := state.WorkingMemory[key]
	if !ok {
		return nil
	}
	s, _ := v.([]string)
	return s
}

// scratchAppendString reads the existing []string at key, appends val, and
// writes the new slice back. Allocates a fresh slice so the original state's
// slice is not aliased — required because NextStep returns state.Clone()
// which shallow-copies the scratch map.
func scratchAppendString(s *core.State, key, val string) {
	if s.WorkingMemory == nil {
		s.WorkingMemory = make(map[string]any, 4)
	}
	existing, _ := s.WorkingMemory[key].([]string)
	out := make([]string, 0, len(existing)+1)
	out = append(out, existing...)
	out = append(out, val)
	s.WorkingMemory[key] = out
}

// latestAssistantText scans state.Messages from the end backwards and returns
// the plain text of the most recent assistant message, or "" if none.
//
// Rules are pure and cannot inspect Event, but they CAN read state.Messages
// (it is part of the state). LearnFromFailure uses this to fold the model's
// reflection / critique text into its reflections slice during the retry phase.
func latestAssistantText(state core.State) string {
	for i := len(state.Messages) - 1; i >= 0; i-- {
		m := state.Messages[i]
		if m.Role != core.ROLE_ASSISTANT {
			continue
		}
		for _, p := range m.Parts {
			if p.Kind == core.PART_KIND_PLAIN_TEXT && p.Text != "" {
				return p.Text
			}
		}
	}
	return ""
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
		CallModel: &core.ModelRequest{
			RequestID: newID(),
			Messages:  state.Messages,
		},
	}
}

func callToolInstruction(call core.ToolCall) core.Instruction {
	return core.Instruction{
		Kind:     core.INSTRUCTION_CALL_TOOL,
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
