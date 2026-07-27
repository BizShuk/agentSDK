package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/security"
	"github.com/bizshuk/agentsdk/reasoning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/tool"
	"github.com/bizshuk/agentsdk/utils/testutil"
)

// cleanupAppDir removes ~/.config/<name> once the test finishes. OpenForCLI
// writes into the real home dir (gosdk/config resolves it internally and
// ignores APP_CONFIG_DIR), so each test uses a distinctive app name and
// tears its directory down.
func cleanupAppDir(t *testing.T, name string) {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(home, ".config", name)) })
}

// echoTool is a trivial tool the scripted provider can call.
type echoTool struct{}

func (echoTool) Name() string { return "echo" }
func (echoTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "echo", Description: "echo the input back", Risk: core.RISK_LEVEL_LOW}
}
func (echoTool) Call(_ context.Context, args json.RawMessage) (core.ToolResult, error) {
	return core.ToolResult{Name: "echo", OK: true, Output: string(args)}, nil
}

// panicTool blows up on call — stands in for any tool bug that would
// otherwise unwind through the engine.
type panicTool struct{}

func (panicTool) Name() string { return "boom" }
func (panicTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "boom", Risk: core.RISK_LEVEL_LOW}
}
func (panicTool) Call(context.Context, json.RawMessage) (core.ToolResult, error) {
	panic("tool exploded")
}

// testAgent is a configurable agent.Runner for the table below. Its
// Bootstrap method owns the complete engine and opening state.
type testAgent struct {
	name          string
	tool          core.Tool
	store         core.StateStore
	bootstrapErr  error
	completeErr   error
	bootstrapRan  bool
	completeRan   bool
	completeState core.State
}

func (a *testAgent) Name() string { return a.name }

func (a *testAgent) Bootstrap(_ context.Context, host *agent.Host) (*runtime.Engine, core.State, error) {
	a.bootstrapRan = true
	if a.bootstrapErr != nil {
		return nil, core.State{}, a.bootstrapErr
	}

	provider := testutil.NewScriptedProvider()
	provider.EnqueueToolCall("call-1", a.tool.Name(), map[string]any{"msg": "hi"})
	provider.EnqueueEndTurn("done")

	reg := tool.NewRegistry()
	reg.Register(a.tool)

	step := reasoning.NewDecide(map[string]reasoning.DecisionRule{
		core.REASON_REACT: reasoning.NewThinkThenAct(),
	})

	engine := runtime.NewEngine(step, provider, reg)
	engine.Store = a.store

	state := core.State{
		RunID:          host.RunID,
		ReasoningStyle: core.REASON_REACT,
		Autonomy:       core.AUTONOMY_L4,
		Budget:         core.Budget{MaxTurns: 5},
	}
	return engine, state, nil
}

func (a *testAgent) OnComplete(_ context.Context, final core.State) error {
	a.completeRan = true
	a.completeState = final
	return a.completeErr
}

// testHost builds a Host for a single-agent.Run call. StateStore and
// WAL are the in-memory fakes from utils/testutil so the test does
// not touch the filesystem.
func testHost(name string) *agent.Host {
	return &agent.Host{
		RunID:      name,
		StateStore: testutil.NewMemStore(),
		WAL:        testutil.NewMemWAL(),
	}
}

func TestRunHappyPath(t *testing.T) {
	const name = "agentsdk-app-test-happy"

	a := &testAgent{name: name, tool: echoTool{}}
	err := agent.Run(context.Background(), a, testHost(name))

	require.NoError(t, err)
	assert.True(t, a.bootstrapRan)
	assert.True(t, a.completeRan, "OnComplete should fire on a successful run")
	assert.Equal(t, core.RUN_STATUS_COMPLETED, a.completeState.Status)
}

func TestCustomRunnerOwnsOpeningState(t *testing.T) {
	const name = "agentsdk-app-test-runner-state"
	cleanupAppDir(t, name)

	a := &testAgent{name: name, tool: echoTool{}}
	err := agent.Run(context.Background(), a, testHost(a.name))

	require.NoError(t, err)
	assert.Equal(t, name, a.completeState.RunID)
}

func TestRunRejectsNilHostBeforeBootstrap(t *testing.T) {
	a := &testAgent{name: "agentsdk-app-test-nil-host", tool: echoTool{}}

	err := agent.Run(context.Background(), a, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "host")
	assert.False(t, a.bootstrapRan)
}

func TestRunRejectsNilRunner(t *testing.T) {
	err := agent.Run(context.Background(), nil, testHost("unused"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "runner")
}

func TestRunRejectsCancelledContextBeforeBootstrap(t *testing.T) {
	a := &testAgent{name: "agentsdk-app-test-cancelled-context", tool: echoTool{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := agent.Run(ctx, a, testHost(a.name))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, a.bootstrapRan)
}

func TestRunBootstrapFailure(t *testing.T) {
	const name = "agentsdk-app-test-bootstrap"
	cleanupAppDir(t, name)

	a := &testAgent{name: name, tool: echoTool{}, bootstrapErr: errors.New("bad wiring")}
	err := agent.Run(context.Background(), a, testHost(a.name))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap")
	assert.False(t, a.completeRan)
}

func TestRunOnCompleteFailureFailsProcess(t *testing.T) {
	const name = "agentsdk-app-test-oncomplete"
	cleanupAppDir(t, name)

	a := &testAgent{name: name, tool: echoTool{}, completeErr: errors.New("publish failed")}
	err := agent.Run(context.Background(), a, testHost(a.name))

	require.Error(t, err, "undelivered results are not a successful run")
	assert.Contains(t, err.Error(), "publish failed")
	assert.True(t, a.completeRan)
}

// TestRunRecoversToolPanic is the load-bearing case: a panicking tool must
// not take the process down silently, and must not leave the run persisted
// as `running` — a later Resume would replay from that stale snapshot.
func TestRunRecoversToolPanic(t *testing.T) {
	const name = "agentsdk-app-test-panic"
	cleanupAppDir(t, name)

	store := testutil.NewMemStore()
	a := &testAgent{name: name, tool: panicTool{}, store: store}

	runErr := agent.Run(context.Background(), a, testHost(a.name))

	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "tool exploded")

	runIDs, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, runIDs, 1, "the crashed run should still be persisted")

	final, err := store.Load(context.Background(), runIDs[0])
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_FAILED, final.Status,
		"a panicked run must be terminal, not left running")
	assert.False(t, a.completeRan, "OnComplete must not fire on a crashed run")
}

func TestRunRejectsEmptyName(t *testing.T) {
	a := &testAgent{name: "", tool: echoTool{}}
	err := agent.Run(context.Background(), a, testHost(a.name))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Name")
	assert.False(t, a.bootstrapRan)
}

// TestRunHonorsTimeout confirms the deadline reaches the Runner — Bootstrap
// sees a ctx that is already bounded, so a provider call inherits it.
func TestRunHonorsTimeout(t *testing.T) {
	const name = "agentsdk-app-test-timeout"
	cleanupAppDir(t, name)

	var gotDeadline bool
	a := &deadlineProbe{
		testAgent: &testAgent{name: name, tool: echoTool{}},
		probe: func(ctx context.Context) {
			_, gotDeadline = ctx.Deadline()
		},
	}

	err := agent.Run(context.Background(), a, testHost(a.name), agent.WithTimeout(2*time.Second))

	require.NoError(t, err)
	assert.True(t, gotDeadline, "Bootstrap should receive a ctx carrying the run deadline")
}

type deadlineProbe struct {
	*testAgent
	probe func(context.Context)
}

func (a *deadlineProbe) Bootstrap(ctx context.Context, host *agent.Host) (*runtime.Engine, core.State, error) {
	a.probe(ctx)
	return a.testAgent.Bootstrap(ctx, host)
}

// --- Interactive seam (Task 3) ---

// alwaysAsk is an ApprovalPolicy that gates every tool call, so a run
// reaches PAUSED_APPROVAL on its first CALL_TOOL.
type alwaysAsk struct{}

func (alwaysAsk) Decide(_ struct{}, _ core.AutonomyLevel, _ core.CallToolInstruction, _ core.ToolSpec) core.ApprovalAction {
	return core.APPROVAL_ACTION_ASK
}

// pausingEngine builds an engine whose first tool call trips the approval
// gate — the run pauses at PAUSED_APPROVAL until a decision arrives.
func pausingEngine() (*runtime.Engine, core.State) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCall("c1", "echo", map[string]any{"msg": "hi"})
	prov.EnqueueEndTurn("done")
	reg := tool.NewRegistry()
	reg.Register(echoTool{})
	step := reasoning.NewDecide(map[string]reasoning.DecisionRule{
		core.REASON_REACT: reasoning.NewThinkThenAct(),
	})
	eng := runtime.NewEngine(step, prov, reg)
	eng.Middleware = middleware.Chain(security.ApprovalGate(alwaysAsk{}))
	return eng, core.State{
		ReasoningStyle: core.REASON_REACT,
		Autonomy:       core.AUTONOMY_L2,
		Budget:         core.Budget{MaxTurns: 10},
	}
}

// chatEngine builds an engine that completes immediately (end_turn), then
// completes again after a follow-up — the ROUND_END path.
func chatEngine() (*runtime.Engine, core.State) {
	prov := testutil.NewScriptedProvider()
	prov.EnqueueEndTurn("first answer")
	prov.EnqueueEndTurn("second answer")
	step := reasoning.NewDecide(map[string]reasoning.DecisionRule{
		core.REASON_REACT: reasoning.NewThinkThenAct(),
	})
	eng := runtime.NewEngine(step, prov, tool.NewRegistry())
	return eng, core.State{ReasoningStyle: core.REASON_REACT, Budget: core.Budget{MaxTurns: 10}}
}

// interactiveAgent implements agent.Interactive; boot picks the engine shape.
type interactiveAgent struct {
	name        string
	boot        func() (*runtime.Engine, core.State)
	next        func(context.Context, agent.Pause) (agent.Resume, error)
	final       core.State
	completeRan bool
}

func (a *interactiveAgent) Name() string { return a.name }

func (a *interactiveAgent) Bootstrap(_ context.Context, host *agent.Host) (*runtime.Engine, core.State, error) {
	eng, st := a.boot()
	eng.Store = host.StateStore
	eng.Log = host.WAL
	st.RunID = host.RunID
	return eng, st, nil
}

func (a *interactiveAgent) NextRound(ctx context.Context, p agent.Pause) (agent.Resume, error) {
	return a.next(ctx, p)
}

func (a *interactiveAgent) OnComplete(_ context.Context, final core.State) error {
	a.completeRan = true
	a.final = final
	return nil
}

// pausingAgent pauses but does NOT implement Interactive — the fallback
// case that must still exit cleanly.
type pausingAgent struct {
	name        string
	final       core.State
	completeRan bool
}

func (a *pausingAgent) Name() string { return a.name }
func (a *pausingAgent) Bootstrap(_ context.Context, host *agent.Host) (*runtime.Engine, core.State, error) {
	eng, st := pausingEngine()
	eng.Store = host.StateStore
	eng.Log = host.WAL
	st.RunID = host.RunID
	return eng, st, nil
}
func (a *pausingAgent) OnComplete(_ context.Context, final core.State) error {
	a.completeRan = true
	a.final = final
	return nil
}

// TestRunConsultsInteractiveOnApprovalPause: an approval pause is routed to
// NextRound, and an APPROVE answer carries the run to COMPLETED.
func TestRunConsultsInteractiveOnApprovalPause(t *testing.T) {
	const name = "agentsdk-app-test-interactive-approve"
	cleanupAppDir(t, name)

	var seen []agent.PauseReason
	a := &interactiveAgent{
		name: name,
		boot: pausingEngine,
		next: func(_ context.Context, p agent.Pause) (agent.Resume, error) {
			seen = append(seen, p.Reason)
			if p.Reason == agent.PAUSE_APPROVAL {
				return agent.Resume{Decision: core.APPROVAL_DECISION_APPROVE, By: "tester"}, nil
			}
			return agent.Resume{Stop: true}, nil
		},
	}
	err := agent.Run(context.Background(), a, testHost(a.name), agent.WithRoundTimeout(5*time.Second))
	require.NoError(t, err)
	assert.Contains(t, seen, agent.PAUSE_APPROVAL)
	assert.True(t, a.completeRan)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, a.final.Status)
}

// TestRunInteractiveRejectCompletesRun: a REJECT answer ends the run
// without the gated tool ever executing.
func TestRunInteractiveRejectCompletesRun(t *testing.T) {
	const name = "agentsdk-app-test-interactive-reject"
	cleanupAppDir(t, name)

	a := &interactiveAgent{
		name: name,
		boot: pausingEngine,
		next: func(_ context.Context, p agent.Pause) (agent.Resume, error) {
			if p.Reason == agent.PAUSE_APPROVAL {
				return agent.Resume{Decision: core.APPROVAL_DECISION_REJECT, By: "tester"}, nil
			}
			return agent.Resume{Stop: true}, nil
		},
	}
	err := agent.Run(context.Background(), a, testHost(a.name))
	require.NoError(t, err)
	assert.True(t, a.completeRan)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, a.final.Status)
}

// TestRunFollowUpReopensCompletedRun: a completed run is offered back to
// the application, which feeds one more input before stopping.
func TestRunFollowUpReopensCompletedRun(t *testing.T) {
	const name = "agentsdk-app-test-interactive-followup"
	cleanupAppDir(t, name)

	var rounds int
	a := &interactiveAgent{
		name: name,
		boot: chatEngine,
		next: func(_ context.Context, p agent.Pause) (agent.Resume, error) {
			rounds++
			assert.Equal(t, agent.PAUSE_ROUND_END, p.Reason)
			if rounds == 1 {
				return agent.Resume{Input: "one more question"}, nil
			}
			return agent.Resume{}, nil // empty input at round_end → stop
		},
	}
	err := agent.Run(context.Background(), a, testHost(a.name))
	require.NoError(t, err)
	assert.Equal(t, 2, rounds, "asked after the first completion and after the follow-up")
	assert.True(t, a.completeRan)
}

// TestRunExitsOnPauseWithoutInteractive: a Runner that pauses but does not
// implement Interactive still exits cleanly, leaving the pause persisted.
func TestRunExitsOnPauseWithoutInteractive(t *testing.T) {
	const name = "agentsdk-app-test-no-interactive"
	cleanupAppDir(t, name)

	a := &pausingAgent{name: name}
	err := agent.Run(context.Background(), a, testHost(a.name))
	require.NoError(t, err, "no Interactive → clean exit")
	assert.True(t, a.completeRan)
	assert.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, a.final.Status,
		"the pause is left for an external verb")
}
