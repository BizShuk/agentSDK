package runtime_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/bizshuk/agentsdk/memory/filestore"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// highRiskTool is a LOW-effort HIGH-risk tool whose Call records that it
// ran, so the e2e can assert the approved call actually executed after
// resume (not just that the run unpause-then-completed).
type highRiskTool struct {
	inner  *action.TypedTool[hrArgs, hrOut]
	called int
}

type hrArgs struct {
	What string `json:"what"`
}

type hrOut struct {
	Fixed string `json:"fixed"`
}

func newHighRiskTool() *highRiskTool {
	t := &highRiskTool{}
	inner := action.NewTypedTool("propose_fix", "propose a high-risk fix",
		func(_ context.Context, a hrArgs) (hrOut, error) {
			t.called++
			return hrOut{Fixed: a.What}, nil
		})
	inner.SetRisk(core.RISK_LEVEL_HIGH)
	t.inner = inner
	return t
}

func (t *highRiskTool) Name() string            { return t.inner.Name() }
func (t *highRiskTool) Description() string     { return t.inner.Description() }
func (t *highRiskTool) Schema() core.ToolSpec   { return t.inner.Schema() }
func (t *highRiskTool) Risk() core.RiskLevel    { return t.inner.Risk() }
func (t *highRiskTool) Call(ctx context.Context, args json.RawMessage) (core.ToolResult, error) {
	return t.inner.Call(ctx, args)
}

// TestM4E2EApprovalPauseThenResumeApprove drives the full HITL story:
//
//  1. ScriptedProvider returns a HIGH-risk tool_use (propose_fix).
//  2. SecureMiddleware's ApprovalGate (L2 + HIGH → ASK) rewrites it to
//     REQUEST_APPROVAL → run pauses with PAUSED_APPROVAL + a
//     PendingApproval carrying the tool call.
//  3. Out-of-band: SubmitHumanDecision(approve) writes the decision into
//     persisted State and resumes.
//  4. consumeApprovedPendingCall re-seeds the approved call; the gate
//     passes it through (approved_call_id); propose_fix executes; the
//     provider then returns end_turn → COMPLETED.
func TestM4E2EApprovalPauseThenResumeApprove(t *testing.T) {
	dir := t.TempDir()
	store, err := filestore.NewJSONFileStateStore(dir)
	require.NoError(t, err)

	prov := testutil.NewScriptedProvider()
	// Turn 1: model requests the high-risk tool.
	prov.EnqueueToolCall("call-1", "propose_fix", map[string]any{"what": "restart db"})
	// Turn 2 (after resume): model is done.
	prov.EnqueueEndTurn("fix applied")

	reg := action.NewRegistry()
	hr := newHighRiskTool()
	reg.Register(hr)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, reg)
	// Full security chain with the real L0-L4 approval policy. Sandbox is
	// nil: propose_fix has no "path"/"command" arg.
	loop.Middleware = config.SecureMiddleware(nil, action.DefaultApprovalPolicy{})
	loop.Approval = action.DefaultApprovalPolicy{}
	loop.Store = store
	loop.Emitter = func(eff core.Instruction) {}

	state := core.State{
		RunID:          "m4-hitl",
		ReasoningStyle: core.REASON_REACT,
		Autonomy:       core.AUTONOMY_L2,
		Budget:         core.Budget{MaxTurns: 20},
	}
	paused, err := loop.Run(context.Background(), state)
	require.NoError(t, err)

	// (1) The run must have paused for approval, not completed.
	assert.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, paused.Status,
		"high-risk call at L2 must pause for approval")
	require.NotEmpty(t, paused.PendingApprovals, "a pending approval must be recorded")
	pa := paused.PendingApprovals[0]
	assert.Equal(t, "propose_fix", pa.ToolCall.Name)
	assert.Equal(t, core.RISK_LEVEL_HIGH, pa.Risk)
	assert.Equal(t, 0, hr.called, "the tool must NOT have executed while paused")

	// Persisted state must carry the pause (approve reads from Store).
	require.NoError(t, store.Save(context.Background(), paused))

	// (2) Out-of-band approval, then resume via SubmitHumanDecision.
	final, err := loop.SubmitHumanDecision(context.Background(), "m4-hitl",
		core.APPROVAL_DECISION_APPROVE, "tester")
	require.NoError(t, err)

	// (3) The approved call executed and the run completed.
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status,
		"after approval the run must complete")
	assert.Equal(t, 1, hr.called, "the approved high-risk tool must execute exactly once")
	assert.Empty(t, final.PendingApprovals, "decided approval must be consumed")
}

// TestM4E2EApprovalRejectTerminates verifies the reject branch: a
// rejected approval short-circuits the run to COMPLETED without ever
// calling the tool.
func TestM4E2EApprovalRejectTerminates(t *testing.T) {
	dir := t.TempDir()
	store, err := filestore.NewJSONFileStateStore(dir)
	require.NoError(t, err)

	prov := testutil.NewScriptedProvider()
	prov.EnqueueToolCall("call-1", "propose_fix", map[string]any{"what": "drop table"})

	reg := action.NewRegistry()
	hr := newHighRiskTool()
	reg.Register(hr)

	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, reg)
	loop.Middleware = config.SecureMiddleware(nil, action.DefaultApprovalPolicy{})
	loop.Approval = action.DefaultApprovalPolicy{}
	loop.Store = store
	loop.Emitter = func(eff core.Instruction) {}

	paused, err := loop.Run(context.Background(), core.State{
		RunID: "m4-reject", ReasoningStyle: core.REASON_REACT,
		Autonomy: core.AUTONOMY_L2, Budget: core.Budget{MaxTurns: 20},
	})
	require.NoError(t, err)
	require.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, paused.Status)
	require.NoError(t, store.Save(context.Background(), paused))

	final, err := loop.SubmitHumanDecision(context.Background(), "m4-reject",
		core.APPROVAL_DECISION_REJECT, "tester")
	require.NoError(t, err)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status,
		"rejected approval must terminate the run")
	assert.Equal(t, 0, hr.called, "the tool must never execute on reject")
}
