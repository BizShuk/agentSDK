package app_test

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

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/app"
	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/internal/testutil"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/security"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
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

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echo the input back" }
func (echoTool) Risk() core.RiskLevel {
	return core.RISK_LEVEL_LOW
}
func (echoTool) Schema() core.ToolSpec {
	return core.ToolSpec{Name: "echo", Description: "echo the input back", Risk: core.RISK_LEVEL_LOW}
}
func (echoTool) Call(_ context.Context, args json.RawMessage) (core.ToolResult, error) {
	return core.ToolResult{Name: "echo", OK: true, Output: string(args)}, nil
}

// panicTool blows up on call — stands in for any tool bug that would
// otherwise unwind through the engine.
type panicTool struct{ echoTool }

func (panicTool) Name() string { return "boom" }
func (panicTool) Schema() core.ToolSpec {
	return core.ToolSpec{Name: "boom", Risk: core.RISK_LEVEL_LOW}
}
func (panicTool) Call(context.Context, json.RawMessage) (core.ToolResult, error) {
	panic("tool exploded")
}

// testAgent is a configurable app.Agent for the table below.
type testAgent struct {
	name          string
	tool          core.Tool
	store         core.StateStore
	preflightErr  error
	bootstrapErr  error
	completeErr   error
	preflightRan  bool
	bootstrapRan  bool
	completeRan   bool
	completeState core.State
}

func (a *testAgent) Name() string { return a.name }

func (a *testAgent) Bootstrap(_ context.Context, _ *config.AppConfig) (*runtime.Engine, core.State, error) {
	a.bootstrapRan = true
	if a.bootstrapErr != nil {
		return nil, core.State{}, a.bootstrapErr
	}

	provider := testutil.NewScriptedProvider()
	provider.EnqueueToolCall("call-1", a.tool.Name(), map[string]any{"msg": "hi"})
	provider.EnqueueEndTurn("done")

	reg := action.NewRegistry()
	reg.Register(a.tool)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})

	engine := runtime.NewEngine(step, provider, reg)
	engine.Store = a.store // nil in most cases — Run backfills from cfg

	state := core.State{
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

// preflightAgent adds the optional Preflighter to testAgent. It is a
// separate type so the other cases stay free of the hook entirely — an
// Agent that does not implement Preflighter must skip that step.
type preflightAgent struct{ *testAgent }

func (a *preflightAgent) Preflight(context.Context, *config.AppConfig) error {
	a.preflightRan = true
	return a.preflightErr
}

func TestRunHappyPath(t *testing.T) {
	const name = "agentsdk-app-test-happy"
	cleanupAppDir(t, name)

	agent := &testAgent{name: name, tool: echoTool{}}
	code := app.Run(context.Background(), agent)

	assert.Equal(t, app.EXIT_OK, code)
	assert.True(t, agent.bootstrapRan)
	assert.True(t, agent.completeRan, "OnComplete should fire on a successful run")
	assert.Equal(t, core.RUN_STATUS_COMPLETED, agent.completeState.Status)
}

func TestRunBackfillsPersistenceFromConfig(t *testing.T) {
	const name = "agentsdk-app-test-backfill"
	cleanupAppDir(t, name)

	// store left nil in Bootstrap → Run must wire cfg.StateStore, so the
	// run ID must be non-empty and the final state must have persisted.
	agent := &testAgent{name: name, tool: echoTool{}}
	code := app.Run(context.Background(), agent)

	require.Equal(t, app.EXIT_OK, code)
	assert.NotEmpty(t, agent.completeState.RunID, "Run should seed RunID from AppConfig")
}

func TestRunPreflightFailureAbortsBeforeBootstrap(t *testing.T) {
	const name = "agentsdk-app-test-preflight"
	cleanupAppDir(t, name)

	base := &testAgent{name: name, tool: echoTool{}, preflightErr: errors.New("no api key")}
	agent := &preflightAgent{testAgent: base}

	code := app.Run(context.Background(), agent)

	assert.Equal(t, app.EXIT_ERROR, code)
	assert.True(t, base.preflightRan)
	assert.False(t, base.bootstrapRan, "a failed preflight must not reach Bootstrap")
}

func TestRunBootstrapFailure(t *testing.T) {
	const name = "agentsdk-app-test-bootstrap"
	cleanupAppDir(t, name)

	agent := &testAgent{name: name, tool: echoTool{}, bootstrapErr: errors.New("bad wiring")}
	code := app.Run(context.Background(), agent)

	assert.Equal(t, app.EXIT_ERROR, code)
	assert.False(t, agent.completeRan)
}

func TestRunOnCompleteFailureFailsProcess(t *testing.T) {
	const name = "agentsdk-app-test-oncomplete"
	cleanupAppDir(t, name)

	agent := &testAgent{name: name, tool: echoTool{}, completeErr: errors.New("publish failed")}
	code := app.Run(context.Background(), agent)

	assert.Equal(t, app.EXIT_ERROR, code, "undelivered results are not a successful run")
	assert.True(t, agent.completeRan)
}

// TestRunRecoversToolPanic is the load-bearing case: a panicking tool must
// not take the process down silently, and must not leave the run persisted
// as `running` — a later Resume would replay from that stale snapshot.
func TestRunRecoversToolPanic(t *testing.T) {
	const name = "agentsdk-app-test-panic"
	cleanupAppDir(t, name)

	store := testutil.NewMemStore()
	agent := &testAgent{name: name, tool: panicTool{}, store: store}

	code := app.Run(context.Background(), agent)

	assert.Equal(t, app.EXIT_ERROR, code)

	runIDs, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, runIDs, 1, "the crashed run should still be persisted")

	final, err := store.Load(context.Background(), runIDs[0])
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_FAILED, final.Status,
		"a panicked run must be terminal, not left running")
	assert.False(t, agent.completeRan, "OnComplete must not fire on a crashed run")
}

func TestRunRejectsEmptyName(t *testing.T) {
	agent := &testAgent{name: "", tool: echoTool{}}
	assert.Equal(t, app.EXIT_ERROR, app.Run(context.Background(), agent))
	assert.False(t, agent.bootstrapRan)
}

// TestRunHonorsTimeout confirms the deadline reaches the Agent — Bootstrap
// sees a ctx that is already bounded, so a provider call inherits it.
func TestRunHonorsTimeout(t *testing.T) {
	const name = "agentsdk-app-test-timeout"
	cleanupAppDir(t, name)

	var gotDeadline bool
	agent := &deadlineProbe{
		testAgent: &testAgent{name: name, tool: echoTool{}},
		probe: func(ctx context.Context) {
			_, gotDeadline = ctx.Deadline()
		},
	}

	code := app.Run(context.Background(), agent, app.WithTimeout(2*time.Second))

	assert.Equal(t, app.EXIT_OK, code)
	assert.True(t, gotDeadline, "Bootstrap should receive a ctx carrying the run deadline")
}

type deadlineProbe struct {
	*testAgent
	probe func(context.Context)
}

func (a *deadlineProbe) Bootstrap(ctx context.Context, cfg *config.AppConfig) (*runtime.Engine, core.State, error) {
	a.probe(ctx)
	return a.testAgent.Bootstrap(ctx, cfg)
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
	reg := action.NewRegistry()
	reg.Register(echoTool{})
	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
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
	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	eng := runtime.NewEngine(step, prov, action.NewRegistry())
	return eng, core.State{ReasoningStyle: core.REASON_REACT, Budget: core.Budget{MaxTurns: 10}}
}

// interactiveAgent implements app.Interactive; boot picks the engine shape.
type interactiveAgent struct {
	name        string
	boot        func() (*runtime.Engine, core.State)
	next        func(context.Context, app.Pause) (app.Resume, error)
	final       core.State
	completeRan bool
}

func (a *interactiveAgent) Name() string { return a.name }

func (a *interactiveAgent) Bootstrap(_ context.Context, _ *config.AppConfig) (*runtime.Engine, core.State, error) {
	eng, st := a.boot()
	return eng, st, nil
}

func (a *interactiveAgent) NextRound(ctx context.Context, p app.Pause) (app.Resume, error) {
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
func (a *pausingAgent) Bootstrap(_ context.Context, _ *config.AppConfig) (*runtime.Engine, core.State, error) {
	eng, st := pausingEngine()
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

	var seen []app.PauseReason
	a := &interactiveAgent{
		name: name,
		boot: pausingEngine,
		next: func(_ context.Context, p app.Pause) (app.Resume, error) {
			seen = append(seen, p.Reason)
			if p.Reason == app.PAUSE_APPROVAL {
				return app.Resume{Decision: core.APPROVAL_DECISION_APPROVE, By: "tester"}, nil
			}
			return app.Resume{Stop: true}, nil
		},
	}
	code := app.Run(context.Background(), a, app.WithRoundTimeout(5*time.Second))
	require.Equal(t, app.EXIT_OK, code)
	assert.Contains(t, seen, app.PAUSE_APPROVAL)
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
		next: func(_ context.Context, p app.Pause) (app.Resume, error) {
			if p.Reason == app.PAUSE_APPROVAL {
				return app.Resume{Decision: core.APPROVAL_DECISION_REJECT, By: "tester"}, nil
			}
			return app.Resume{Stop: true}, nil
		},
	}
	code := app.Run(context.Background(), a)
	require.Equal(t, app.EXIT_OK, code)
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
		next: func(_ context.Context, p app.Pause) (app.Resume, error) {
			rounds++
			assert.Equal(t, app.PAUSE_ROUND_END, p.Reason)
			if rounds == 1 {
				return app.Resume{Input: "one more question"}, nil
			}
			return app.Resume{}, nil // empty input at round_end → stop
		},
	}
	code := app.Run(context.Background(), a)
	require.Equal(t, app.EXIT_OK, code)
	assert.Equal(t, 2, rounds, "asked after the first completion and after the follow-up")
	assert.True(t, a.completeRan)
}

// TestRunExitsOnPauseWithoutInteractive: an Agent that pauses but does not
// implement Interactive still exits cleanly, leaving the pause persisted.
func TestRunExitsOnPauseWithoutInteractive(t *testing.T) {
	const name = "agentsdk-app-test-no-interactive"
	cleanupAppDir(t, name)

	a := &pausingAgent{name: name}
	code := app.Run(context.Background(), a)
	require.Equal(t, app.EXIT_OK, code, "no Interactive → clean exit")
	assert.True(t, a.completeRan)
	assert.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, a.final.Status,
		"the pause is left for an external verb")
}
