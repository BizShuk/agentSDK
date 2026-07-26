// Package security hosts the safety middlewares that intercept tool
// calls. sandbox_mw.go applies a Sandbox policy; spotlight_mw.go
// marks untrusted tool output; sanitizer_mw.go strips prompt-injection.
package security

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/tool"
)

// Sandbox returns a Middleware that consults the given Sandbox before
// allowing CALL_TOOL through. Denied calls are rewritten as
// EFFECT_NOTIFY (level=error) followed by EFFECT_DONE so the run
// stops cleanly rather than continuing with a phantom tool result.
func Sandbox(policy tool.Sandbox) middleware.Middleware {
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
			if eff.Kind != core.INSTRUCTION_CALL_TOOL || eff.CallTool == nil {
				return next(ctx, state, eff)
			}
			v := policy.Check(eff.CallTool.Call.Name, eff.CallTool.Call.Args)
			if v == tool.VERDICT_ALLOW {
				return next(ctx, state, eff)
			}
			// Denied — emit NOTIFY first so the operator sees the reason,
			// then DONE so the run stops.
			denyEff := core.Instruction{
				Kind: core.INSTRUCTION_NOTIFY,
				Notify: &core.NotifyInstruction{
					Level:   "error",
					Message: fmt.Sprintf("sandbox denied tool %q with args %v", eff.CallTool.Call.Name, eff.CallTool.Call.Args),
				},
			}
			s, _, _, err := next(ctx, state, denyEff)
			if err != nil {
				return s, nil, false, err
			}
			doneEff := core.Instruction{Kind: core.INSTRUCTION_DONE}
			s2, _, term, err := next(ctx, s, doneEff)
			if err != nil {
				return s2, nil, false, err
			}
			return s2, nil, term, nil
		}
	}
}