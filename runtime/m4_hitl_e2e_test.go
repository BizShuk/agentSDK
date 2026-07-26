package runtime_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/agent/permission"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/memory/filestore"
	"github.com/bizshuk/agentsdk/middleware/preset"
	"github.com/bizshuk/agentsdk/reasoning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/tool"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type hrArgs struct {
	What string `json:"what"`
}

type hrOut struct {
	Fixed string `json:"fixed"`
}

func registerHighRiskTool(reg *tool.Registry, called *int) {
	tool.RegisterFunc(reg, "propose_fix", "propose a high-risk fix", core.RISK_LEVEL_HIGH,
		func(_ context.Context, a hrArgs) (hrOut, error) {
			*called++
			return hrOut{Fixed: a.What}, nil
		})
}

// TestM4E2EApprovalPauseThenResumeApprove drives the full HITL story:
//
//  1. ScriptedProvider returns a HIGH-risk tool_use (propose_fix).
//  2. preset.Secure's ApprovalGate (L2 + HIGH → ASK) rewrites it to
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

	reg := tool.NewRegistry()
	called1 := 0
	registerHighRiskTool(reg, &called1)

	step := reasoning.NewDecide(map[string]reasoning.DecisionRule{
		core.REASON_REACT: reasoning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, reg)
	// Full security chain with the real L0-L4 approval policy. Sandbox is
	// nil: propose_fix has no "path"/"command" arg.
	loop.Middleware = preset.Secure(nil, permission.DefaultApprovalPolicy{})
	loop.Approval = permission.DefaultApprovalPolicy{}
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
	assert.Equal(t, 0, called1, "the tool must NOT have executed while paused")

	// Persisted state must carry the pause (approve reads from Store).
	require.NoError(t, store.Save(context.Background(), paused))

	// (2) Out-of-band approval, then resume via SubmitHumanDecision.
	final, err := loop.SubmitHumanDecision(context.Background(), "m4-hitl",
		core.APPROVAL_DECISION_APPROVE, "tester")
	require.NoError(t, err)

	// (3) The approved call executed and the run completed.
	assert.Equal(t, core.RUN_STATUS_COMPLETED, final.Status,
		"after approval the run must complete")
	assert.Equal(t, 1, called1, "the approved high-risk tool must execute exactly once")
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

	reg := tool.NewRegistry()
	called2 := 0
	registerHighRiskTool(reg, &called2)

	step := reasoning.NewDecide(map[string]reasoning.DecisionRule{
		core.REASON_REACT: reasoning.NewThinkThenAct(),
	})
	loop := runtime.NewEngine(step, prov, reg)
	loop.Middleware = preset.Secure(nil, permission.DefaultApprovalPolicy{})
	loop.Approval = permission.DefaultApprovalPolicy{}
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
	assert.Equal(t, 0, called2, "rejected tool must NOT execute")
}
