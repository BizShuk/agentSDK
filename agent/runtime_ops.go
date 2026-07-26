package agent

import (
	"context"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// NewEngine is the embeddable seam for callers that need to assemble an
// Engine at the L2 boundary. They give agent the components (decide,
// provider, tool registry); agent owns the runtime container.
//
// L1 callers (samples) that build Engines themselves used to import the
// runtime package directly — that is a reverse dependency on L3. Routing
// the construction through agent keeps the boundary clean while keeping
// the runtime type itself unchanged.
func NewEngine(decide core.Decide, provider core.Provider, reg core.ToolRegistry) *Engine {
	return newRuntimeEngine(decide, provider, reg)
}

// Perform drives the engine for one round-trip: backfills the
// persistence and run ID from host, then runs. It is the L1-facing
// version of agent.Run: same surface, no Runner, no Bootstrap.
func Perform(ctx context.Context, host *Host, e *Engine, state core.State) (core.State, error) {
	if e == nil {
		return core.State{}, errNilEngine
	}
	if e.Store == nil && host != nil {
		e.Store = host.StateStore
	}
	if e.Log == nil && host != nil {
		e.Log = host.WAL
	}
	if state.RunID == "" && host != nil {
		state.RunID = host.RunID
	}
	return e.Run(ctx, state)
}

// ResumeRun replays the persisted WAL for runID, returning the final
// state. The engine's Store and Log are backfilled from host when nil.
// A nil host leaves them as the caller set them.
//
// The trailing "Run" suffix keeps it clear next to the Resume struct
// that agent.Interactive returns — same package, two unrelated shapes.
func ResumeRun(ctx context.Context, host *Host, e *Engine, runID string) (core.State, error) {
	if e == nil {
		return core.State{}, errNilEngine
	}
	if e.Store == nil && host != nil {
		e.Store = host.StateStore
	}
	if e.Log == nil && host != nil {
		e.Log = host.WAL
	}
	return e.Resume(ctx, runID)
}

// ListRuns lists the runIDs persisted in host's StateStore. A nil host
// surfaces as a no-op (empty slice) so callers do not need a fork for
// "no persistence configured".
func ListRuns(ctx context.Context, host *Host) ([]string, error) {
	if host == nil || host.StateStore == nil {
		return nil, nil
	}
	return host.StateStore.List(ctx)
}

// Approve persists an approval decision out-of-band. It is the seam for
// a subcommand (or a second binary) that decides a PendingApproval
// written by another agent without driving its engine. The actual
// handoff to the engine happens via runtime.Engine.SubmitHumanDecision
// on the next Run/Resume; L2 just guarantees the persisted state carries
// the decision.
//
// PendingApproval.Decided is encoded as "Decision set + DecidedAt
// populated". The seam writes both — Decision alone would let a paused
// run resume with an undecided-looking approval.
func Approve(ctx context.Context, host *Host, runID string, decision core.ApprovalDecision, by string) error {
	if host == nil || host.StateStore == nil {
		return errNoHost
	}
	state, err := host.StateStore.Load(ctx, runID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	changed := false
	for i := range state.PendingApprovals {
		pa := &state.PendingApprovals[i]
		if pa.DecidedAt != nil {
			continue
		}
		pa.Decision = decision
		pa.DecidedAt = &now
		pa.DecidedBy = by
		changed = true
	}
	if !changed {
		return nil
	}
	return host.StateStore.Save(ctx, state)
}
