// Package app is the composition root for CLI-shaped agents.
//
// It owns the process lifecycle — signal binding, config load, preflight,
// wall-clock timeout, panic recovery, structured run logging, and exit
// codes — so a binary reduces to:
//
//	func main() { app.Main(&myAgent{}) }
//
// app introduces no new abstraction over runtime.Engine. It is boilerplate
// convergence: every knob it sets has a default that the Agent can override,
// and Bootstrap hands back the live *runtime.Engine so callers keep full
// access to Middleware / Store / Approval / Emitter. Presets, not walls.
//
// Opinionated application conventions (cron scheduling, per-folder fan-out,
// audit trails) do NOT belong here — they belong to the layer above.
package app

import (
	"context"

	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/runtime"
)

// Agent is the contract a binary implements. Two methods, both required.
//
// Name is the application identifier. It feeds gosdk/config (which resolves
// ~/.config/<Name>) and every log record, so it must be stable across runs
// of the same binary — it is the key under which state and WAL persist.
//
// Bootstrap assembles the run. It receives the AppConfig that Run already
// opened (data dir, log dir, run ID, file-backed StateStore + WAL) and
// returns the Engine to drive plus the State to drive it from. Bootstrap is
// where the agent picks its provider, registers tools, chooses a reasoning
// style and middleware chain, and seeds the opening messages.
//
// Bootstrap runs AFTER config load and preflight, so it may read settings
// and may assume credentials have been validated. Returning an error aborts
// the run with exit code 1 before any model call is issued.
//
// Store and Log on the returned Engine are pre-wired from cfg when the agent
// leaves them nil; an agent that binds its own persistence keeps it.
type Agent interface {
	Name() string
	Bootstrap(ctx context.Context, cfg *config.AppConfig) (*runtime.Engine, core.State, error)
}

// Preflighter is an optional Agent extension. When implemented, Preflight
// runs after config load and before Bootstrap — the place to fail fast on
// missing credentials or unreachable dependencies.
//
// The point is to surface a misconfiguration BEFORE the run leaves any
// trace: no state file, no WAL, no half-finished conversation. An agent
// that discovers a bad API key on its first model call has already created
// a run that will sit in `running` forever.
type Preflighter interface {
	Preflight(ctx context.Context, cfg *config.AppConfig) error
}

// Completer is an optional Agent extension. When implemented, OnComplete
// runs after the Engine returns, receiving the final State — the seam for
// reporting, notification, or publishing results.
//
// It runs only on a successful run (Engine returned no error). An error
// from OnComplete fails the process (exit 1): a run whose results could not
// be delivered did not succeed, whatever the loop thinks.
type Completer interface {
	OnComplete(ctx context.Context, final core.State) error
}
