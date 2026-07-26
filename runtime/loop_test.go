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
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/harness"
	"github.com/bizshuk/agentsdk/middleware/security"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/utils/testutil"
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
//
//	FakeProvider queues (1) end_turn → expect loop to exit on DONE.
func TestReActEndTurnExitsLoop(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("done")

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})

	loop := runtime.NewEngine(step, prov, action.NewRegistry())
	loop.Emitter = func(eff core.Instruction) {}
	state := core.State{
		RunID:          "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 5},
	}

	final, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	assert.Equal(t, 1, prov.RequestCount())
}

// TestReActOneToolCall exercises CALL_TOOL → CALL_MODEL → DONE:
//  1. ReAct starts in THINK → emits CALL_MODEL
//  2. FakeProvider returns tool_use (add 2+3) → loop dispatches tool, gets result 5
//  3. Pattern advances to OBSERVE → emits CALL_MODEL
//  4. FakeProvider returns end_turn → loop exits DONE
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
		RunID:          "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 10},
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
		RunID:          "r1",
		ReasoningStyle: core.REASON_PLAN_THEN_RUN,
		Budget:         core.Budget{MaxTurns: 5},
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
		RunID:          "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 3},
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
		RunID:          "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 5},
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

	// ChooseAgent emits a NOTIFY in its select phase when an agent list is
	// seeded, then a CALL_MODEL in delegate; the scripted end_turn short-
	// circuits the run to COMPLETED.
	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_PICK_AGENT: planning.NewChooseAgent(),
	})

	notifier := &testutil.RecordingNotifier{}
	loop := runtime.NewEngine(step, prov, action.NewRegistry())
	loop.Notifier = notifier
	loop.Emitter = func(eff core.Instruction) {}

	state := core.State{
		RunID:          "r1",
		ReasoningStyle: core.REASON_PICK_AGENT,
		Budget:         core.Budget{MaxTurns: 3},
	}
	// Seed an agent list so the select phase emits NOTIFY ("router chose agent: x").
	planning.SeedAgents(&state, []string{"x"})
	_, err := loop.Run(context.Background(), state)
	require.NoError(t, err)
	assert.NotEmpty(t, notifier.Messages())
	assert.Contains(t, notifier.Messages()[0], "router chose agent")
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
		RunID:          "r1",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 5},
	}

	_, err := loop.RunWithEvent(context.Background(), state, core.Event{
		Kind: core.EVENT_OBSERVATION,
		Observation: &core.Observation{
			ID: "p1", Source: "test",
			ObservedAt: time.Now().UTC(),
			Payload:    "wake up",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, prov.RequestCount())
}

// silence unused imports if json/never gets used
var _ = json.Marshal
var _ = errors.New
var _ = harness.IsBudgetExceeded

// --- batch settlement (one round, N tool calls) ---

// toolResultsOf collects every ROLE_TOOL part in transcript order, which
// is the only view that matters for the settlement invariant: an
// assistant turn carrying N tool_use parts must be followed by N
// tool_result messages before the next CALL_MODEL.
func toolResultsOf(s core.State) []core.ToolResultPart {
	var out []core.ToolResultPart
	for _, m := range s.Messages {
		if m.Role != core.ROLE_TOOL {
			continue
		}
		for _, p := range m.Parts {
			if p.Kind == core.PART_KIND_TOOL_RESULT && p.ToolResult != nil {
				out = append(out, *p.ToolResult)
			}
		}
	}
	return out
}

// batchEngine wires the shared setup for the batch tests: an `add` tool,
// ReAct, an always-allow approval stub.
func batchEngine(t *testing.T, prov *testutil.ScriptedProvider) *runtime.Engine {
	t.Helper()
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
	eng := runtime.NewEngine(step, prov, reg)
	eng.Approval = stubApproval{}
	return eng
}

// TestMultiToolCallBatchSettlesEveryCall pins the settlement invariant.
//
// Before batch dispatch the engine seeded ToolCalls[0] and silently
// dropped the rest, while the assistant message already carried all
// three tool_use parts — a transcript Anthropic-format providers reject
// with 400, and which every model reads as "two calls still running".
func TestMultiToolCallBatchSettlesEveryCall(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCalls(
		core.ToolCall{ID: "c1", Name: "add", Args: map[string]any{"n1": 1, "n2": 1}},
		core.ToolCall{ID: "c2", Name: "add", Args: map[string]any{"n1": 2, "n2": 2}},
		core.ToolCall{ID: "c3", Name: "add", Args: map[string]any{"n1": 3, "n2": 3}},
	)
	prov.EnqueueEndTurn("all three done")

	final, err := batchEngine(t, prov).Run(context.Background(), core.State{
		RunID:          "batch-1",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 10},
	})
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)

	got := toolResultsOf(final)
	require.Len(t, got, 3, "every tool_use part needs its own tool_result")
	for i, want := range []string{"c1", "c2", "c3"} {
		assert.Equal(t, want, got[i].CallID, "results must stay in request order")
		assert.True(t, got[i].OK, "call %s should have executed", want)
	}
}

// TestBatchSettlesCallsSkippedByPause covers the mid-batch terminal case:
// an approval pause on the second call means the third never dispatches,
// so it must be settled explicitly rather than left without a result.
func TestBatchSettlesCallsSkippedByPause(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCalls(
		core.ToolCall{ID: "c1", Name: "add", Args: map[string]any{"n1": 1, "n2": 1}},
		core.ToolCall{ID: "c2", Name: "add", Args: map[string]any{"n1": 2, "n2": 2}},
		core.ToolCall{ID: "c3", Name: "add", Args: map[string]any{"n1": 3, "n2": 3}},
	)

	eng := batchEngine(t, prov)
	// Ask on the second call only, so the batch pauses partway through.
	eng.Middleware = middleware.Chain(security.ApprovalGate(askOnCall{id: "c2"}))

	final, err := eng.Run(context.Background(), core.State{
		RunID:          "batch-2",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 10},
	})
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, final.Status)

	got := toolResultsOf(final)
	require.Len(t, got, 2, "c1 executed, c3 settled as skipped; c2 is the pending approval")
	assert.Equal(t, "c1", got[0].CallID)
	assert.True(t, got[0].OK)
	assert.Equal(t, "c3", got[1].CallID)
	assert.False(t, got[1].OK)
	assert.Contains(t, got[1].Error, "skipped")
}

// askOnCall asks for approval on exactly one call id and allows the rest.
type askOnCall struct{ id string }

func (a askOnCall) Decide(_ struct{}, _ core.AutonomyLevel, inst core.CallToolInstruction, _ core.ToolSpec) core.ApprovalAction {
	if inst.Call.ID == a.id {
		return core.APPROVAL_ACTION_ASK
	}
	return core.APPROVAL_ACTION_ALLOW
}

// budgetBatch is the four-call over-budget batch shared by the tool-call
// budget tests: MaxToolCalls=2, model asks for four.
func budgetBatchProvider() *testutil.ScriptedProvider {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCalls(
		core.ToolCall{ID: "c1", Name: "add", Args: map[string]any{"n1": 1, "n2": 1}},
		core.ToolCall{ID: "c2", Name: "add", Args: map[string]any{"n1": 2, "n2": 2}},
		core.ToolCall{ID: "c3", Name: "add", Args: map[string]any{"n1": 3, "n2": 3}},
		core.ToolCall{ID: "c4", Name: "add", Args: map[string]any{"n1": 4, "n2": 4}},
	)
	return prov
}

// TestToolCallBudgetSkipsWholeBatchAndPauses: when a batch exceeds
// MaxToolCalls the entire list is skipped — not partially executed — and
// the run pauses on a continue-gate approval so a human decides whether
// to resume. Every tool_use part is still settled so the transcript stays
// valid.
func TestToolCallBudgetSkipsWholeBatchAndPauses(t *testing.T) {
	prov := budgetBatchProvider()

	eng := batchEngine(t, prov)
	eng.Store = testutil.NewMemStore()

	final, err := eng.Run(context.Background(), core.State{
		RunID:          "budget-pause",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 10, MaxToolCalls: 2},
	})
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, final.Status)

	got := toolResultsOf(final)
	require.Len(t, got, 4, "the whole batch is settled, none executed")
	for _, r := range got {
		assert.False(t, r.OK, "%s must be skipped, not run", r.CallID)
		assert.Contains(t, r.Error, "tool call budget")
	}

	require.Len(t, final.PendingApprovals, 1)
	pa := final.PendingApprovals[0]
	assert.Equal(t, "tool_call_budget", pa.Reason)
	assert.Nil(t, pa.ToolCall, "a continue-gate carries no specific call")
	assert.Equal(t, 1, prov.RequestCount(), "only the batch round ran; no tool executed")
}

// TestToolCallBudgetResumeApprove: approving the continue-gate resumes the
// run — the model is called again, reads the skipped results, and this
// time ends the turn.
func TestToolCallBudgetResumeApprove(t *testing.T) {
	prov := budgetBatchProvider()
	prov.EnqueueEndTurn("understood, smaller batch next time")

	eng := batchEngine(t, prov)
	eng.Store = testutil.NewMemStore()

	paused, err := eng.Run(context.Background(), core.State{
		RunID:          "budget-approve",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 10, MaxToolCalls: 2},
	})
	require.NoError(t, err)
	require.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, paused.Status)

	final, err := eng.SubmitHumanDecision(context.Background(), "budget-approve",
		core.APPROVAL_DECISION_APPROVE, "operator")
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	assert.Equal(t, 2, prov.RequestCount(), "one batch round + one resume round")
	assert.Empty(t, final.PendingApprovals, "the gate was consumed")
}

// TestToolCallBudgetResumeReject: rejecting the continue-gate ends the run
// without another model call.
func TestToolCallBudgetResumeReject(t *testing.T) {
	prov := budgetBatchProvider()

	eng := batchEngine(t, prov)
	eng.Store = testutil.NewMemStore()

	_, err := eng.Run(context.Background(), core.State{
		RunID:          "budget-reject",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxTurns: 10, MaxToolCalls: 2},
	})
	require.NoError(t, err)

	final, err := eng.SubmitHumanDecision(context.Background(), "budget-reject",
		core.APPROVAL_DECISION_REJECT, "operator")
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status)
	assert.Equal(t, 1, prov.RequestCount(), "reject → no resume model call")
}

// TestRoundBudgetTripsOnCallModel: with MaxRounds=2 and a model that
// never ends its turn, the third CALL_MODEL is refused by the Budget
// middleware with Reason "round_budget".
func TestRoundBudgetTripsOnCallModel(t *testing.T) {
	prov := testutil.NewScriptedProvider()
	for range 5 {
		prov.EnqueueToolCall("loop", "add", map[string]any{"n1": 1, "n2": 1})
	}

	eng := batchEngine(t, prov)
	eng.Middleware = middleware.Chain(harness.Budget())

	_, err := eng.Run(context.Background(), core.State{
		RunID:          "round-1",
		ReasoningStyle: core.REASON_REACT,
		Budget:         core.Budget{MaxRounds: 2},
	})
	require.Error(t, err)
	var be *harness.BudgetExceededError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "round_budget", be.Reason)
}
