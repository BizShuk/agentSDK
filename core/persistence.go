package core

import "context"

// StateStore persists State. Implementations must be safe for concurrent use
// across runs; RunID is the namespace.
//
// The file-backed default lives in memory/filestore.JSONFileStateStore.
type StateStore interface {
	Save(ctx context.Context, state State) error
	Load(ctx context.Context, runID string) (State, error)
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, runID string) error
}

// WriteAheadLog is the append-only event log used for crash recovery. Recovery
// replays events from sinceSeq without reissuing model calls.
type WriteAheadLog interface {
	Append(ctx context.Context, runID string, seq int, event Event) error
	Read(ctx context.Context, runID string, sinceSeq int) ([]Event, error)
	TruncateFrom(ctx context.Context, runID string, uptoSeq int) error
}
