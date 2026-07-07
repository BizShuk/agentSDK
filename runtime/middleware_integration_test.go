package runtime_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/internal/testutil"
	"github.com/bizshuk/agentsdk/memory/filestore"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/harness"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRuntimeLoopguardTripInRealtime exercises loopguard via the Loop
// using a custom pattern that emits CALL_TOOLs back-to-back (without
// interleaving CALL_MODEL — which would reset the counter). The guard
// must surface REQUEST_APPROVAL on the 5th consecutive call.
func TestRuntimeLoopguardTripInRealtime(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	// Provider is irrelevant — pattern bypasses LLM entirely.

	reg := action.NewRegistry()
	noop := action.NewTypedTool("noop", "no-op",
		func(_ context.Context, _ struct{}) (struct{}, error) { return struct{}{}, nil })
	noop.RiskV = core.RISK_LEVEL_LOW
	reg.Register(noop)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: &stuckPattern{},
	})
	loop := runtime.NewEngine(step, prov, reg)
	loop.Approval = stubApproval{}
	loop.Emitter = func(eff core.Instruction) {}

	state := core.State{
		RunID: "r1", ReasoningStyle: core.REASON_REACT,
		Budget: core.Budget{MaxTurns: 20},
	}
	final, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, final.Status)
	require.NotEmpty(t, final.PendingApprovals)
	assert.Equal(t, "loop_detected", final.PendingApprovals[0].Reason)
}

// stuckPattern emits CALL_TOOL on every Decide — never calls a model.
// Used to verify loopguard catches consecutive tool repeats even when
// no CALL_MODEL intervenes to reset the counter.
type stuckPattern struct{}

func (s *stuckPattern) Kind() core.ReasoningStyle { return core.REASON_REACT }
func (s *stuckPattern) NextStep(state core.State) (core.State, []core.Instruction) {
	return state, []core.Instruction{
		{Kind: core.INSTRUCTION_CALL_TOOL,
		CallTool: &core.CallToolInstruction{
			Call: core.ToolCall{ID: "c", Name: "noop", Args: map[string]any{}},
		},
	}}
}

// TestRuntimeBudgetExceededExitsRun verifies the budget middleware's
// integration with the runtime: when Budget.UsedTurns >= MaxTurns, the
// loop exits with a BudgetExceededError wrapped by IsBudgetExceeded.
func TestRuntimeBudgetExceededExitsRun(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	for i := 0; i < 100; i++ {
		prov.EnqueueToolCall("c", "noop", map[string]any{})
	}
	reg := action.NewRegistry()
	noop := action.NewTypedTool("noop", "no-op",
		func(_ context.Context, _ struct{}) (struct{}, error) { return struct{}{}, nil })
	noop.RiskV = core.RISK_LEVEL_LOW
	reg.Register(noop)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, reg)
	loop.Approval = stubApproval{}
	loop.Emitter = func(eff core.Instruction) {}

	state := core.State{
		RunID: "r1", ReasoningStyle: core.REASON_REACT,
		Budget: core.Budget{MaxTurns: 3},
	}
	final, err := loop.Run(context.Background(), state)
	require.Error(t, err)
	assert.True(t, runtime.IsBudgetExceeded(err))
	assert.Equal(t, core.RUN_STATUS_FAILED, final.Status)
}

// TestRuntimeResumeFromWAL exercises the recovery path: start a run with
// Store + WAL, then Resume and verify it finishes with the FakeProvider's
// remaining scripted results.
func TestRuntimeResumeFromWAL(t *testing.T) {
	dir := t.TempDir()
	store, err := filestore.NewJSONFileStateStore(dir)
	require.NoError(t, err)
	wal, err := filestore.NewJSONLFileLog(dir)
	require.NoError(t, err)

	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("done")

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_PICK_AGENT: planning.NewChooseAgent(),
	})
	loop := runtime.NewEngine(step, prov, action.NewRegistry())
	loop.Middleware = identityChain()
	loop.Store = store
	loop.Log = wal
	loop.Emitter = func(eff core.Instruction) {}

	// First run — Router stub emits one NOTIFY + DONE.
	state := core.State{
		RunID: "r1", ReasoningStyle: core.REASON_PICK_AGENT,
		Status: core.RUN_STATUS_RUNNING,
		Budget: core.Budget{MaxTurns: 5},
		LastInputSeq: 0,
	}
	_, err = loop.Run(context.Background(), state)
	require.NoError(t, err)

	// Verify state was persisted.
	replayed, err := wal.Read(context.Background(), "r1", 0)
	require.NoError(t, err)
	_ = replayed

	// Resume — must complete cleanly.
	_, err = loop.Resume(context.Background(), "r1")
	require.NoError(t, err)
}

// identityChain is a no-op middleware chain — useful to disable the
// DefaultMiddleware for tests that don't want loopguard / budget side effects.
func identityChain() middleware.Middleware {
	return func(next middleware.Next) middleware.Next { return next }
}

// silence unused imports if changed
var _ = harness.Budget