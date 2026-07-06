package testutil

import (
	"context"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// MemStore is an in-memory StateStore for tests.
type MemStore struct {
	mu     sync.RWMutex
	states map[string]core.State
}

// NewMemStore returns an empty store.
func NewMemStore() *MemStore { return &MemStore{states: make(map[string]core.State)} }

// Save implements core.StateStore.
func (s *MemStore) Save(_ context.Context, st core.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[st.RunID] = st.Clone()
	return nil
}

// Load implements core.StateStore.
func (s *MemStore) Load(_ context.Context, runID string) (core.State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.states[runID]
	if !ok {
		return core.State{}, ErrNotFound
	}
	return st.Clone(), nil
}

// List implements core.StateStore.
func (s *MemStore) List(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.states))
	for k := range s.states {
		out = append(out, k)
	}
	return out, nil
}

// Delete implements core.StateStore.
func (s *MemStore) Delete(_ context.Context, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, runID)
	return nil
}

// ErrNotFound is returned when Load cannot find the runID.
var ErrNotFound = &storeError{"run not found"}

type storeError struct{ msg string }

func (e *storeError) Error() string { return e.msg }

// MemWAL is an in-memory WAL for tests.
type MemWAL struct {
	mu     sync.Mutex
	byRun  map[string][]core.Input
}

// NewMemWAL returns an empty WAL.
func NewMemWAL() *MemWAL { return &MemWAL{byRun: make(map[string][]core.Input)} }

// Append implements core.WAL.
func (w *MemWAL) Append(_ context.Context, runID string, seq int, in core.Input) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.byRun[runID] = append(w.byRun[runID], in)
	return nil
}

// Replay implements core.WAL.
func (w *MemWAL) Replay(_ context.Context, runID string, sinceSeq int) ([]core.Input, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	all := w.byRun[runID]
	if sinceSeq >= len(all) {
		return nil, nil
	}
	out := append([]core.Input(nil), all[sinceSeq:]...)
	return out, nil
}

// Truncate implements core.WAL.
func (w *MemWAL) Truncate(_ context.Context, runID string, uptoSeq int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	all := w.byRun[runID]
	if uptoSeq >= len(all) {
		return nil
	}
	w.byRun[runID] = append([]core.Input(nil), all[uptoSeq:]...)
	return nil
}