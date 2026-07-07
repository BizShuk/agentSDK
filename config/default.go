// Package config provides the one-stop AppConfig for CLI samples.
// It wraps gosdk/config initialization, log file setup, and filestore-backed
// StateStore + WriteAheadLog wiring in a single OpenForCLI call.
//
// DefaultMiddleware lives here (rather than runtime/) because it wires
// concrete harness/loopguard implementations — importing those from
// runtime would create a dependency cycle with middleware/.
package config

import (
	"time"

	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/harness"
	"github.com/bizshuk/agentsdk/middleware/loopguard"
)

// DefaultMiddleware returns the M2 chain: retry → timeout → budget → loopguard.
// Order matches plans/plan-only-and-plan-breezy-pike.md.
//
// Callers wire this explicitly:
//
//	loop.Middleware = config.DefaultMiddleware()
//
// When Engine.Middleware is nil, the runtime defaults to middleware.Identity()
// (no-op) — the caller is responsible for choosing a chain.
func DefaultMiddleware() middleware.Middleware {
	return middleware.Chain(
		harness.Retry(harness.RetryConfig{N: 3, BaseBackoff: 100 * time.Millisecond, MaxBackoff: 5 * time.Second}),
		harness.Timeout(harness.TimeoutConfig{PerEffect: 60 * time.Second}),
		harness.Budget(),
		loopguard.New(loopguard.Config{MaxRepeats: 5}),
	)
}
