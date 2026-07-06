// Package middleware holds the cross-cutting chain that wraps effect
// dispatch in runtime.Loop.
//
// Composition order (outer → inner) per plans/plan-only-and-plan-breezy-pike.md:
//
//	tracing (M3) → retry → timeout → budget → loopguard →
//	sandbox (M3) → approval (M4) → spotlight/sanitizer (M3) →
//	base dispatcher
//
// M2 implements retry, timeout, budget, loopguard. The other layers are
// stubs / placeholders for later milestones — wiring the chain now lets
// later milestones slot in without changing runtime.Loop.
package middleware

import (
	"context"

	"github.com/bizshuk/agentsdk/core"
)

// Next is the dispatcher signature a Middleware wraps.
//
// Each Middleware may:
//   - inspect/modify eff before passing it down (e.g. loopguard may
//     rewrite CALL_TOOL into REQUEST_APPROVAL)
//   - wrap the inner call with retries / timeouts / context
//   - synthesize the returned Input on behalf of inner (e.g. budget DENY)
//   - propagate, suppress, or rewrite errors
//
// The wrapped dispatch must preserve two invariants of the runtime:
//
//  1. non-nil (state, *input) ⇒ the input is foldable on the next iteration
//  2. terminal=true ⇒ the runtime returns immediately with state.Status
type Next func(ctx context.Context, state core.State, eff core.Effect) (core.State, *core.Input, bool, error)

// Dispatcher is the terminal Next — what the chain wraps when it has
// nothing else to do. In runtime.Loop, this is the function that calls
// ModelProvider / ToolRegistry / Notifier.
type Dispatcher func(ctx context.Context, state core.State, eff core.Effect) (core.State, *core.Input, bool, error)

// Middleware transforms a Next into another Next.
type Middleware func(Next) Next

// Chain composes middlewares outer-to-inner. The first argument is the
// outermost; the last wraps the dispatcher.
//
// Example:
//
//	loop.Middleware = middleware.Chain(
//	    harness.Retry{N: 3},
//	    harness.Timeout{PerEffect: 30 * time.Second},
//	    harness.Budget{},
//	    loopguard.New(loopguard.Config{MaxRepeats: 5}),
//	)
func Chain(mws ...Middleware) Middleware {
	return func(final Next) Next {
		for i := len(mws) - 1; i >= 0; i-- {
			final = mws[i](final)
		}
		return final
	}
}

// Identity is a no-op Middleware — useful for tests and for default wiring
// when callers want to attach nothing.
func Identity() Middleware {
	return func(next Next) Next { return next }
}