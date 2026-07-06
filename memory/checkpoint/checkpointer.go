// Package checkpoint owns the durability protocol — when to snapshot
// State, when to WAL an Input, how to rebuild a run from disk.
package checkpoint

import (
	"context"
	"fmt"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// Checkpointer pairs a StateStore (per-run snapshot) and a WAL
// (per-run append-only Inputs). It exposes the two operations the
// runtime needs:
//
//   Checkpoint(ctx, s) → persist the current State snapshot
//   Recover(ctx, runID) → load State, replay Inputs since LastInputSeq
//
// Recover returns the rebuilt State and the ordered Inputs that were
// applied to it. The runtime can re-feed those Inputs to Step / dispatch
// without re-invoking the LLM — that is the entire point.
type Checkpointer struct {
	Store core.StateStore
	WAL   core.WAL

	mu sync.Mutex // serialize Checkpoint / Recover against concurrent calls
}

// New constructs a Checkpointer from any (Store, WAL) pair.
func New(store core.StateStore, wal core.WAL) *Checkpointer {
	return &Checkpointer{Store: store, WAL: wal}
}

// Checkpoint persists State and (best-effort) WALs a marker so the WAL
// boundary matches the State boundary.
//
// In M2 we keep it minimal: save State; the WAL is appended on each
// Input fold (runtime side). M3 may introduce explicit "checkpoint
// record" entries to make recovery robust against torn writes.
func (c *Checkpointer) Checkpoint(ctx context.Context, s core.State) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Store == nil {
		return fmt.Errorf("checkpoint: nil store")
	}
	return c.Store.Save(ctx, s)
}

// RecoverResult bundles the rebuilt state and the inputs that drove it
// past the saved snapshot.
type RecoverResult struct {
	State  core.State
	Inputs []core.Input
}

// Recover rebuilds a run. Strategy:
//
//  1. Load State from Store.
//  2. Replay Inputs from WAL since State.LastInputSeq.
//  3. Return both — caller re-feeds Inputs in order; pattern advances
//     through scratch identically because the State carried forward is
//     identical to the pre-crash snapshot.
//
// The runtime MUST NOT re-issue model calls during replay — the WAL
// already contains the original ModelResults / ToolResults.
func (c *Checkpointer) Recover(ctx context.Context, runID string) (RecoverResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Store == nil {
		return RecoverResult{}, fmt.Errorf("recover: nil store")
	}
	s, err := c.Store.Load(ctx, runID)
	if err != nil {
		return RecoverResult{}, fmt.Errorf("recover load state: %w", err)
	}
	if c.WAL == nil {
		return RecoverResult{State: s}, nil
	}
	inputs, err := c.WAL.Replay(ctx, runID, s.LastInputSeq)
	if err != nil {
		return RecoverResult{}, fmt.Errorf("recover replay: %w", err)
	}
	return RecoverResult{State: s, Inputs: inputs}, nil
}