package planning

import (
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// nowOrZero returns state.UpdatedAt if zero, else time.Now. Patterns use
// this to stamp UpdatedAt consistently without bringing in a clock injection
// at every call site (Test providers inject a clock via Budget.NowFunc in M2).
func nowOrZero(state core.State) time.Time {
	if state.UpdatedAt.IsZero() {
		return time.Now().UTC()
	}
	return state.UpdatedAt
}

func scratchString(state core.State, key, def string) string {
	if state.Scratch == nil {
		return def
	}
	v, ok := state.Scratch[key]
	if !ok {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func scratchInt(state core.State, key string, def int) int {
	if state.Scratch == nil {
		return def
	}
	v, ok := state.Scratch[key]
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
	if state.Scratch == nil {
		return core.ToolCall{}, false
	}
	v, ok := state.Scratch[key]
	if !ok {
		return core.ToolCall{}, false
	}
	tc, ok := v.(core.ToolCall)
	return tc, ok
}

func scratchBlueprint(state core.State) ([]core.ToolCall, bool) {
	if state.Scratch == nil {
		return nil, false
	}
	v, ok := state.Scratch[PE_BLUEPRINT_STEPS]
	if !ok {
		return nil, false
	}
	arr, ok := v.([]core.ToolCall)
	return arr, ok
}

func scratchSet(s *core.State, key string, val any) {
	if s.Scratch == nil {
		s.Scratch = make(map[string]any, 4)
	}
	s.Scratch[key] = val
}

func callModelFromMessages(state core.State) core.Effect {
	return core.Effect{
		Kind: core.EFFECT_CALL_MODEL,
		CallModel: &core.CallModelEffect{
			RequestID: newID(),
			Messages:  state.Messages,
		},
	}
}

func callToolEffect(call core.ToolCall) core.Effect {
	return core.Effect{
		Kind: core.EFFECT_CALL_TOOL,
		CallTool: &core.CallToolEffect{Call: call},
	}
}

func doneEffect() core.Effect {
	return core.Effect{Kind: core.EFFECT_DONE}
}

// hasOKPrefix is the cheap "did the critique approve?" predicate.
// Override by populating scratch with a different prefix if needed.
func hasOKPrefix(s string) bool {
	if len(s) < 3 {
		return false
	}
	return s[:3] == "OK:"
}

// newID is a process-local id generator. Deterministic tests can replace
// this by setting scratch explicitly; id only appears on emitted effects.
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
