// Package checkpoint owns the durability protocol — when to snapshot
// State, when to WAL an Event, how to rebuild a run from disk.
package checkpoint

import (
	"context"
	"fmt"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// Recoverer pairs a StateStore (per-run snapshot) and a WriteAheadLog
// (per-run append-only Events). It exposes the two operations the
// runtime needs:
//
//   Save(ctx, s)    → persist the current State snapshot
//   Recover(ctx, runID) → load State, replay Events since LastInputSeq
//
// Recover returns the rebuilt State and the ordered Events that were
// applied to it. The runtime can re-feed those Events to Decide /
// dispatch without re-invoking the LLM — that is the entire point.
type Recoverer struct {
	Store core.StateStore
	Log   core.WriteAheadLog

	mu sync.Mutex // serialize Save / Recover against concurrent calls
}

// NewRecoverer constructs a Recoverer from any (Store, Log) pair.
func NewRecoverer(store core.StateStore, log core.WriteAheadLog) *Recoverer {
	return &Recoverer{Store: store, Log: log}
}

// Save persists State. The Log is appended on each Event fold (runtime
// side). M3 may introduce explicit "checkpoint record" entries to make
// recovery robust against torn writes.
func (r *Recoverer) Save(ctx context.Context, s core.State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Store == nil {
		return fmt.Errorf("checkpoint: nil store")
	}
	return r.Store.Save(ctx, s)
}

// RecoveredRun bundles the rebuilt state and the events that drove it
// past the saved snapshot.
type RecoveredRun struct {
	State  core.State
	Events []core.Event
}

// Recover rebuilds a run. Strategy:
//
//  1. Load State from Store.
//  2. Read Events from Log since State.LastInputSeq.
//  3. Return both — caller re-feeds Events in order; rule advances
//     through working memory identically because the State carried
//     forward is identical to the pre-crash snapshot.
//
// The runtime MUST NOT re-issue model calls during replay — the Log
// already contains the original ModelResults / ToolResults.
func (r *Recoverer) Recover(ctx context.Context, runID string) (RecoveredRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Store == nil {
		return RecoveredRun{}, fmt.Errorf("recover: nil store")
	}
	s, err := r.Store.Load(ctx, runID)
	if err != nil {
		return RecoveredRun{}, fmt.Errorf("recover load state: %w", err)
	}
	if r.Log == nil {
		return RecoveredRun{State: s}, nil
	}
	events, err := r.Log.Read(ctx, runID, s.LastInputSeq)
	if err != nil {
		return RecoveredRun{}, fmt.Errorf("recover replay: %w", err)
	}
	return RecoveredRun{State: s, Events: events}, nil
}
