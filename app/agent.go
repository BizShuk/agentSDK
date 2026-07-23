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

// PauseReason classifies why a run stopped without the application being
// done with it. It is the discriminator on Pause.
type PauseReason string

const (
	// PAUSE_APPROVAL — the run holds an undecided PendingApproval. This
	// covers both a per-call approval gate and the continue-gate raised
	// when a whole tool batch is skipped over the tool-call budget.
	PAUSE_APPROVAL PauseReason = "approval"
	// PAUSE_ROUND_END — the run reached COMPLETED and would exit, but an
	// interactive application may still have a follow-up to add.
	PAUSE_ROUND_END PauseReason = "round_end"
)

// Pause is what Run hands the application when a run stops but is not yet
// finished with. Reason says why; State is the run at the moment it
// stopped (PendingApprovals populated when Reason is PAUSE_APPROVAL).
type Pause struct {
	State  core.State
	Reason PauseReason
}

// Resume is the application's answer to a Pause.
//
// Decision is read only when Reason == PAUSE_APPROVAL; an empty value
// there is treated as REJECT, because "no answer" must never be read as
// consent for a call the policy already flagged.
//
// Input is appended as a user message before the next round, whatever the
// reason — approving a call AND adding a correction is one round trip.
//
// Stop ends the run immediately. At PAUSE_ROUND_END an empty Input with
// Stop=false also ends it: nothing to add means done.
//
// By attributes the decision in the audit trail (PendingApproval.DecidedBy).
type Resume struct {
	Decision core.ApprovalDecision
	Input    string
	Stop     bool
	By       string
}

// Interactive is the single seam for everything a run needs from the
// application mid-flight: approval decisions AND follow-up input. They are
// the same question — "the run stopped and is not terminal, what next?" —
// asked at different pause reasons, so they are one method rather than the
// three (pause / resolve / reject) an earlier draft split them into.
//
// The Agent owns the input side and decides where the answer comes from:
// stdin, an HTTP endpoint, a Kafka topic, a policy lookup, a channel fed
// by Sink callbacks. Notification, audit, and rollback belong INSIDE
// NextRound; they need the same ctx and the same State and would earn
// nothing as separate interfaces.
//
// Implementations MUST honor ctx cancellation: Run blocks here until an
// answer arrives or the process is asked to stop (SIGINT/SIGTERM,
// WithRoundTimeout, or WithTimeout).
//
// An Agent that does not implement Interactive keeps today's behavior:
// Run returns on the pause and the persisted PendingApprovals are left for
// an out-of-process verb (e.g. `approve --run-id`) to decide.
type Interactive interface {
	NextRound(ctx context.Context, p Pause) (Resume, error)
}
