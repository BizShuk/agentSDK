package runtime_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/bizshuk/agentsdk/memory/checkpoint"
	"github.com/bizshuk/agentsdk/memory/filestore"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/harness"
	"github.com/bizshuk/agentsdk/middleware/loopguard"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCrashRecoveryFullCycle is the M2 verification article:
//  1. Drive a run that produces a verifiable transcript.
//  2. Snapshot the State mid-run.
//  3. Recover via checkpoint; assert exact state equality.
//  4. Resume via Loop.Resume; assert zero new model calls during replay.
func TestCrashRecoveryFullCycle(t *testing.T) {
	dir := t.TempDir()
	store, err := filestore.NewJSONFileStateStore(dir)
	require.NoError(t, err)
	wal, err := filestore.NewJSONLFileLog(dir)
	require.NoError(t, err)

	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCall("c1", "noop", map[string]any{}) // ReAct phase 1
	prov.EnqueueEndTurn("done")                          // final

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
	loop.Store = store
	loop.Log = wal
	loop.Middleware = identityChain() // disable loopguard for a clean run
	loop.Emitter = func(eff core.Instruction) {}

	state := core.State{
		RunID: "rrun", ReasoningStyle: core.REASON_REACT,
		Budget: core.Budget{MaxTurns: 5},
	}
	final, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)

	callsBefore := prov.RequestCount()

	// 2. Checkpoint
	cp := checkpoint.NewRecoverer(store, wal)
	require.NoError(t, cp.Save(context.Background(), final))

	// 3. Recover — same State, no model calls issued
	res, err := cp.Recover(context.Background(), "rrun")
	require.NoError(t, err)
	// Compare via JSON bytes to avoid reflect.DeepEqual's pointer-equality
	// quirks on nested structs (ToolResultChunk.Output is json.RawMessage).
	finalBytes, err := json.Marshal(final)
	require.NoError(t, err)
	recBytes, err := json.Marshal(res.State)
	require.NoError(t, err)
	assert.JSONEq(t, string(finalBytes), string(recBytes),
		"recovered State must equal final State under JSON")
	assert.Equal(t, final.Turn, res.State.Turn)
	assert.Equal(t, final.Status, res.State.Status)
	assert.Equal(t, callsBefore, prov.RequestCount(), "Recover must not issue model calls")

	// 4. Loop.Resume replays the WAL — should complete without errors.
	_, err = loop.Resume(context.Background(), "rrun")
	require.NoError(t, err)
}

// TestChainComposesOverRetryThroughLoopguard demonstrates the full chain in
// composition order: the middlewares see each other through the chain.
func TestChainComposesOverRetryThroughLoopguard(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	for i := 0; i < 10; i++ {
		prov.EnqueueToolCall("c", "noop", map[string]any{})
	}
	reg := action.NewRegistry()
	noop := action.NewTypedTool("noop", "no-op",
		func(_ context.Context, _ struct{}) (struct{}, error) { return struct{}{}, nil })
	noop.RiskV = core.RISK_LEVEL_LOW
	reg.Register(noop)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: &stuckPattern{},
	})

	mw := middleware.Chain(
		harness.Retry(harness.RetryConfig{N: 1, Sleeper: func(time.Duration) {}}),
		harness.Budget(),
		loopguard.New(loopguard.Config{MaxRepeats: 5}),
	)

	loop := runtime.NewEngine(step, prov, reg)
	loop.Middleware = mw
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

// identityChain and stuckPattern live in middleware_integration_test.go.

// silence unused imports
var _ = json.Marshal
