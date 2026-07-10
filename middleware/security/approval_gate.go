package security

import (
	"context"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// ApprovalGate is the middleware that consults an ApprovalPolicy for
// every CALL_TOOL and either passes through (ALLOW), rewrites as
// REQUEST_APPROVAL (ASK — pauses the run), or rewrites as NOTIFY
// error (DENY).
//
// Autonomy is read from state on every dispatch (it is per-run, not
// per-chain), so the policy verdict tracks the run's current trust
// level even if it changes mid-run. Place this in the chain BEFORE the
// base dispatcher so it can short-circuit. The policy-driven decision
// happens on the *outbound* side; once a CALL_TOOL becomes
// REQUEST_APPROVAL, runtime loops naturally pause on PAUSED_APPROVAL
// until SubmitApproval reissues with a fresh APPROVAL_DECISION input.
func ApprovalGate(policy core.ApprovalPolicy) middleware.Middleware {
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
			if eff.Kind != core.INSTRUCTION_CALL_TOOL || eff.CallTool == nil {
				return next(ctx, state, eff)
			}
			// If this call was already approved out-of-band (via
			// SubmitHumanDecision / the `approve` CLI), runtime seeded its
			// id into working memory — let it through without re-asking,
			// otherwise an approved high-risk call would loop forever.
			if approved, ok := state.WorkingMemory["think_then_act.approved_call_id"].(string); ok && approved == eff.CallTool.Call.ID {
				return next(ctx, state, eff)
			}
			// The tool's risk comes from the tool definition; the
			// autonomy comes from the run's state (per-run, mutable).
			schema := core.ToolSpec{
				Name: eff.CallTool.Call.Name,
				Risk: eff.CallTool.Call.Risk,
			}
			verdict := policy.Decide(struct{}{}, state.Autonomy, *eff.CallTool, schema)
			switch verdict {
			case core.APPROVAL_ACTION_ALLOW:
				return next(ctx, state, eff)

			case core.APPROVAL_ACTION_ASK:
				// Rewrite into REQUEST_APPROVAL. The runtime's existing
				// CALL_TOOL → REQUEST_APPROVAL handler will set
				// state.Status = PAUSED_APPROVAL automatically when it
				// sees the rewritten effect.
				approveEff := core.Instruction{
					Kind: core.INSTRUCTION_REQUEST_APPROVAL,
					RequestApproval: &core.RequestApprovalInstruction{
						ApprovalID: "auto-" + eff.CallTool.Call.ID,
						Reason:     "policy_L" + itoa(int(state.Autonomy)) + "_" + string(eff.CallTool.Call.Risk),
						Risk:       eff.CallTool.Call.Risk,
						Summary:    "approval gate: " + eff.CallTool.Call.Name,
						ToolCall:   &eff.CallTool.Call,
					},
				}
				return next(ctx, state, approveEff)

			case core.APPROVAL_ACTION_DENY:
				// Notify the operator; let the run continue (no state
				// change for now). Future versions may also append a
				// synthetic tool_result with OK=false.
				denyEff := core.Instruction{
					Kind: core.INSTRUCTION_NOTIFY,
					Notify: &core.NotifyInstruction{
						Level:   "error",
						Message: "approval denied: " + eff.CallTool.Call.Name,
					},
				}
				return next(ctx, state, denyEff)
			}
			return next(ctx, state, eff)
		}
	}
}