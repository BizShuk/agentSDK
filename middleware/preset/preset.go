// Package preset holds the named middleware chains an application picks
// from: Default for the harness layers, Secure for harness plus the
// security layers.
//
// A preset is a chain, not a menu. The ORDER of these layers is a
// correctness property — retry outermost so it re-runs the whole inner
// stack, sanitizer innermost so it rewrites tool output before anything
// else sees it — which is why config selects a preset by name rather than
// composing layers itself.
//
// It lives under middleware/ rather than in runtime/ because it wires the
// concrete harness, loopguard, and security implementations; runtime
// importing those would cycle back through middleware. The subpackage
// direction (preset → middleware) does not.
package preset

import (
	"time"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/harness"
	"github.com/bizshuk/agentsdk/middleware/loopguard"
	"github.com/bizshuk/agentsdk/middleware/security"
)

// Default returns the M2 chain: retry → timeout → budget → loopguard.
// Order matches plans/plan-only-and-plan-breezy-pike.md.
//
// Callers wire this explicitly:
//
//	loop.Middleware = preset.Default()
//
// When Engine.Middleware is nil, the runtime defaults to middleware.Identity()
// (no-op) — the caller is responsible for choosing a chain.
//
// This chain has NO security layer (sandbox / approval / spotlight /
// sanitizer). Use Secure for production runs that execute tools
// against untrusted or user-facing input.
func Default() middleware.Middleware {
	return middleware.Chain(
		harness.Retry(harness.RetryConfig{N: 3, BaseBackoff: 100 * time.Millisecond, MaxBackoff: 5 * time.Second}),
		harness.Timeout(harness.TimeoutConfig{PerEffect: 60 * time.Second}),
		harness.Budget(),
		loopguard.New(loopguard.Config{MaxRepeats: 5}),
	)
}

// Secure returns the full M3+M4 chain: the M2 harness layers
// plus sandbox → approval → spotlight → sanitizer, wrapping the base
// dispatcher.
//
// Composition (outer → inner) per plans/plan-only-and-plan-breezy-pike.md:
//
//	retry → timeout → budget → loopguard →
//	sandbox → approval → spotlight → sanitizer → base
//
// The approval gate consults `policy` for every CALL_TOOL, reading the
// run's AutonomyLevel from state (per-run). Pass action.DefaultApprovalPolicy{}
// for the L0-L4 enterprise grid, or a custom policy. sandboxPolicy gates
// path/command args; pass action.DefaultPolicy() for the standard
// denylist + /tmp allowlist.
//
// Nil policy disables the approval gate (every tool call passes); nil
// sandboxPolicy disables the sandbox. Both nil is equivalent to
// Default() plus spotlight/sanitizer.
func Secure(sandboxPolicy action.Sandbox, approval core.ApprovalPolicy) middleware.Middleware {
	chain := []middleware.Middleware{
		harness.Retry(harness.RetryConfig{N: 3, BaseBackoff: 100 * time.Millisecond, MaxBackoff: 5 * time.Second}),
		harness.Timeout(harness.TimeoutConfig{PerEffect: 60 * time.Second}),
		harness.Budget(),
		loopguard.New(loopguard.Config{MaxRepeats: 5}),
	}
	if sandboxPolicy != nil {
		chain = append(chain, security.Sandbox(sandboxPolicy))
	}
	if approval != nil {
		chain = append(chain, security.ApprovalGate(approval))
	}
	// spotlight (outer) → sanitizer (inner): sanitizer rewrites injection
	// text first on the return path, then spotlight wraps the result.
	chain = append(chain,
		security.Spotlight(),
		security.DefaultSanitizer().Middleware(),
	)
	return middleware.Chain(chain...)
}
