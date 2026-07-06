// Package security hosts the safety middlewares that intercept tool
// calls. sandbox_mw.go applies a Sandbox policy; spotlight_mw.go
// marks untrusted tool output; sanitizer_mw.go strips prompt-injection.
package security

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// Sandbox returns a Middleware that consults the given Sandbox before
// allowing CALL_TOOL through. Denied calls are rewritten as
// EFFECT_NOTIFY (level=error) followed by EFFECT_DONE so the run
// stops cleanly rather than continuing with a phantom tool result.
func Sandbox(policy action.Sandbox) middleware.Middleware {
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
			if eff.Kind != core.EFFECT_CALL_TOOL || eff.CallTool == nil {
				return next(ctx, state, eff)
			}
			v := policy.Check(eff.CallTool.Call.Name, eff.CallTool.Call.Args)
			if v == action.VERDICT_ALLOW {
				return next(ctx, state, eff)
			}
			// Denied — emit NOTIFY first so the operator sees the reason,
			// then DONE so the run stops.
			denyEff := core.Effect{
				Kind: core.EFFECT_NOTIFY,
				Notify: &core.NotifyEffect{
					Level:   "error",
					Message: fmt.Sprintf("sandbox denied tool %q with args %v", eff.CallTool.Call.Name, eff.CallTool.Call.Args),
				},
			}
			s, _, _, err := next(ctx, state, denyEff)
			if err != nil {
				return s, nil, false, err
			}
			doneEff := core.Effect{Kind: core.EFFECT_DONE}
			s2, _, term, err := next(ctx, s, doneEff)
			if err != nil {
				return s2, nil, false, err
			}
			return s2, nil, term, nil
		}
	}
}