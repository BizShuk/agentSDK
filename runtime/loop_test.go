package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/internal/testutil"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/harness"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addArgs / addOut — the typed shape for our test tool.
type addArgs struct {
	N1 int `json:"n1"`
	N2 int `json:"n2"`
}
type addOut struct {
	Sum int `json:"sum"`
}

// stubApproval always allows.
type stubApproval struct{}

func (stubApproval) Decide(_ struct{}, _ core.AutonomyLevel, _ core.CallToolInstruction, _ core.ToolSpec) core.ApprovalAction {
	return core.APPROVAL_ACTION_ALLOW
}

// TestReActOneToolCall drives a single read-then-end loop:
//   FakeProvider queues (1) end_turn → expect loop to exit on DONE.
func TestReActEndTurnExitsLoop(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("done")

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})

	loop := runtime.NewEngine(step, prov, action.NewRegistry())
	loop.Emitter = func(eff core.Instruction) {}
	state := core.State{
		RunID:        "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:       core.Budget{MaxTurns: 5},
	}

	final, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	assert.Equal(t, 1, prov.RequestCount())
}

// TestReActOneToolCall exercises CALL_TOOL → CALL_MODEL → DONE:
//   1. ReAct starts in THINK → emits CALL_MODEL
//   2. FakeProvider returns tool_use (add 2+3) → loop dispatches tool, gets result 5
//   3. Pattern advances to OBSERVE → emits CALL_MODEL
//   4. FakeProvider returns end_turn → loop exits DONE
func TestReActOneToolCallThenEnd(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCall("c1", "add", map[string]any{"n1": 2, "n2": 3})
	prov.EnqueueEndTurn("the sum is 5")

	reg := action.NewRegistry()
	addTool := action.NewTypedTool("add", "add two ints",
		func(_ context.Context, a addArgs) (addOut, error) {
			return addOut{Sum: a.N1 + a.N2}, nil
		})
	addTool.RiskV = core.RISK_LEVEL_LOW
	reg.Register(addTool)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})

	loop := runtime.NewEngine(step, prov, reg)
	loop.Approval = stubApproval{}
	loop.Emitter = func(eff core.Instruction) {}
	state := core.State{
		RunID:        "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:       core.Budget{MaxTurns: 10},
	}

	final, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	assert.Equal(t, 2, prov.RequestCount())
	// The transcript should contain the tool message with the sum.
	foundToolMsg := false
	for _, m := range final.Messages {
		if m.Role == core.ROLE_TOOL {
			require.Len(t, m.Parts, 1)
			assert.Equal(t, core.PART_KIND_TOOL_RESULT, m.Parts[0].Kind)
			require.NotNil(t, m.Parts[0].ToolResult)
			assert.True(t, m.Parts[0].ToolResult.OK)
			foundToolMsg = true
		}
	}
	assert.True(t, foundToolMsg)
}

// TestPlannerExecutorDispatchesBlueprint drives the seeded blueprint path:
// no CALL_MODEL at all — the blueprint is installed via scratch and
// the pattern dispatches every step, then DONE.
func TestPlannerExecutorDispatchesBlueprint(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	// Two steps; neither requires an LLM call (blueprint seeded via scratch).
	prov.EnqueueEndTurn("unused")

	reg := action.NewRegistry()
	noop := action.NewTypedTool("noop", "no-op",
		func(_ context.Context, a addArgs) (addOut, error) { return addOut{}, nil })
	noop.RiskV = core.RISK_LEVEL_LOW
	reg.Register(noop)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_PLAN_THEN_RUN: planning.NewPlanThenRun(),
	})

	state := core.State{
		RunID:        "r1",
		ReasoningStyle: core.REASON_PLAN_THEN_RUN,
		Budget:       core.Budget{MaxTurns: 5},
	}
	planning.SeedBlueprint(&state, []core.ToolCall{
		{ID: "s1", Name: "noop", Args: map[string]any{}},
		{ID: "s2", Name: "noop", Args: map[string]any{}},
	})

	loop := runtime.NewEngine(step, prov, reg)
	loop.Approval = stubApproval{}
	loop.Emitter = func(eff core.Instruction) {}

	final, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	// Two tool calls were dispatched, no model calls needed (blueprint seeded).
	assert.Equal(t, 0, prov.RequestCount())
}

// TestBudgetExceededStopsLoop verifies the budget guard.
//
// We feed a stream of tool_use results so ReAct never sees an end_turn —
// each iteration dispatches a CALL_TOOL, gets a result, returns to MODEL,
// which produces another tool_use. The loop keeps running until the
// turn budget trips.
func TestBudgetExceededStopsLoop(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	for i := 0; i < 100; i++ {
		prov.EnqueueToolCall(fmt.Sprintf("c%d", i), "noop", map[string]any{})
	}

	reg := action.NewRegistry()
	noop := action.NewTypedTool("noop", "no-op",
		func(_ context.Context, a addArgs) (addOut, error) { return addOut{}, nil })
	noop.RiskV = core.RISK_LEVEL_LOW
	reg.Register(noop)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})

	loop := runtime.NewEngine(step, prov, reg)
	loop.Middleware = middleware.Chain(harness.Budget())
	loop.Approval = stubApproval{}
	loop.Emitter = func(eff core.Instruction) {}
	state := core.State{
		RunID:        "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:       core.Budget{MaxTurns: 3},
	}

	final, err := loop.Run(context.Background(), state)
	require.Error(t, err)
	assert.Equal(t, core.RUN_STATUS_FAILED, final.Status)
	assert.Contains(t, err.Error(), "budget exceeded")
}

// TestStoreAndWAL are wired through the loop and round-trip via MemStore / MemWAL.
func TestStoreAndWAL(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("done")

	store := testutil.NewMemStore()
	wal := testutil.NewMemWAL()

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, action.NewRegistry())
	loop.Emitter = func(eff core.Instruction) {}
	loop.Store = store
	loop.Log = wal

	state := core.State{
		RunID:        "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:       core.Budget{MaxTurns: 5},
	}
	final, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	require.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)

	// State persisted
	loaded, err := store.Load(context.Background(), "r1")
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, loaded.Status)
	assert.Equal(t, final.Turn, loaded.Turn)

	// WAL recorded at least one input
	replayed, err := wal.Read(context.Background(), "r1", 0)
	require.NoError(t, err)
	assert.NotEmpty(t, replayed)
}

// TestNotifyIsCalled ensures the Notifier is invoked for INSTRUCTION_NOTIFY.
func TestNotifyIsCalled(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("done")

	// Use the Router stub which emits NOTIFY + DONE in one Decide call.
	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_PICK_AGENT: planning.NewChooseAgent(),
	})

	notifier := &testutil.RecordingNotifier{}
	loop := runtime.NewEngine(step, prov, action.NewRegistry())
	loop.Notifier = notifier
	loop.Emitter = func(eff core.Instruction) {}

	state := core.State{
		RunID:        "r1",
		ReasoningStyle: core.REASON_PICK_AGENT,
		Budget:       core.Budget{MaxTurns: 3},
	}
	_, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	assert.NotEmpty(t, notifier.Messages())
	assert.Contains(t, notifier.Messages()[0], "STUB")
}

// TestRunWithInputSeedsFirstTurn lets a test inject an input directly.
func TestRunWithInputSeedsFirstTurn(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("ack")

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, action.NewRegistry())
	loop.Emitter = func(eff core.Instruction) {}

	state := core.State{
		RunID:        "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:       core.Budget{MaxTurns: 5},
	}

	_, err := loop.RunWithEvent(context.Background(), state, core.Event{
		Kind: core.EVENT_OBSERVATION,
		Observation: &core.Observation{
			ID: "p1", Source: "test",
			ObservedAt: time.Now().UTC(),
			Payload: "wake up",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, prov.RequestCount())
}

// silence unused imports if json/never gets used
var _ = json.Marshal
var _ = errors.New
var _ = harness.IsBudgetExceeded