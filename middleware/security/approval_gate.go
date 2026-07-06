package security

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// ApprovalGate is the middleware that consults an ApprovalPolicy for
// every CALL_TOOL and either passes through (ALLOW), rewrites as
// REQUEST_APPROVAL (ASK — pauses the run), or rewrites as NOTIFY
// error (DENY).
//
// Place this in the chain BEFORE the base dispatcher so it can short-
// circuit. The policy-driven decision happens on the *outbound* side;
// once a CALL_TOOL becomes REQUEST_APPROVAL, runtime loops naturally
// pause on PAUSED_APPROVAL until SubmitApproval reissues with a fresh
// APPROVAL_DECISION input.
func ApprovalGate(autonomy core.AutonomyLevel, policy core.ApprovalPolicy) middleware.Middleware {
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
			if eff.Kind != core.EFFECT_CALL_TOOL || eff.CallTool == nil {
				return next(ctx, state, eff)
			}
			// Look up the tool's schema in the (small) registry the
			// runtime hands us. ApprovalPolicy needs the Risk field.
			schema := core.ToolSchema{
				Name: eff.CallTool.Call.Name,
				Risk: eff.CallTool.Call.Risk,
			}
			verdict := policy.Decide(struct{}{}, autonomy, *eff.CallTool, schema)
			switch verdict {
			case core.APPROVAL_ACTION_ALLOW:
				return next(ctx, state, eff)

			case core.APPROVAL_ACTION_ASK:
				// Rewrite into REQUEST_APPROVAL. The runtime's existing
				// CALL_TOOL → REQUEST_APPROVAL handler will set
				// state.Status = PAUSED_APPROVAL automatically when it
				// sees the rewritten effect.
				approveEff := core.Effect{
					Kind: core.EFFECT_REQUEST_APPROVAL,
					RequestApproval: &core.RequestApprovalEffect{
						ApprovalID: "auto-" + eff.CallTool.Call.ID,
						Reason:     "policy_" + fmt.Sprint(int(autonomy)) + "_" + string(eff.CallTool.Call.Risk),
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
				denyEff := core.Effect{
					Kind: core.EFFECT_NOTIFY,
					Notify: &core.NotifyEffect{
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