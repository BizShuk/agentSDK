package agent

import (
	"errors"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
)

// newRuntimeEngine is the L2-internal alias used by NewEngine. It exists
// in its own file to keep runtime_ops.go runtime-free and to give grep
// one place that resolves "where does agent build a runtime.Engine".
func newRuntimeEngine(decide core.Decide, provider core.Provider, reg core.ToolRegistry) *Engine {
	return runtime.NewEngine(decide, provider, reg)
}

// ReActStep returns a Decide dispatcher with only the think-then-act
// rule registered. It is the canonical step for L1 callers (samples)
// that wire an Engine without managing their own reasoning vocabulary —
// the underlying implementation is L3, but samples never see the import.
//
// For multi-rule dispatch or non-default reasoning styles, callers
// compose their own core.NewDecide map directly.
func ReActStep() core.Decide {
	return core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
}

// errNilEngine and errNoHost are sentinel errors raised by the L2
// seams; they live next to the constructor so any caller that imports
// runtime_ops is two files away from the full shape.
var (
	errNilEngine = errors.New("agent: engine must not be nil")
	errNoHost    = errors.New("agent: host must not be nil for this operation")
)
