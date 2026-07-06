// Package filestore is the default file-backed StateStore + WAL.
//
// Layout (under baseDir):
//
//	baseDir/
//	├── states/<runID>.json          # State snapshot — last-known
//	└── wal/<runID>.jsonl            # Append-only Inputs (one per line)
//
// Operations are atomic per-file (write-temp + rename for state, append for
// WAL). Concurrent runs share the baseDir but never the same file.
package filestore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// FileStateStore persists State as pretty-printed JSON.
type FileStateStore struct {
	BaseDir string // typically config.GetAppDataDir() / "states"

	mu sync.Mutex
}

// NewFileStateStore returns a store rooted at baseDir. The directory is
// created if missing.
func NewFileStateStore(baseDir string) (*FileStateStore, error) {
	full := filepath.Join(baseDir, "states")
	if err := os.MkdirAll(full, 0o750); err != nil {
		return nil, fmt.Errorf("filestore: mkdir states: %w", err)
	}
	return &FileStateStore{BaseDir: full}, nil
}

func (s *FileStateStore) path(runID string) string {
	return filepath.Join(s.BaseDir, runID+".json")
}

// Save writes State atomically (write-temp + rename).
func (s *FileStateStore) Save(_ context.Context, st core.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("state marshal: %w", err)
	}
	tmp := s.path(st.RunID) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o640); err != nil {
		return fmt.Errorf("state write tmp: %w", err)
	}
	return os.Rename(tmp, s.path(st.RunID))
}

// Load reads a State. Returns an error if the run is unknown.
func (s *FileStateStore) Load(_ context.Context, runID string) (core.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path(runID))
	if err != nil {
		if os.IsNotExist(err) {
			return core.State{}, fmt.Errorf("run not found: %s", runID)
		}
		return core.State{}, err
	}
	var st core.State
	if err := json.Unmarshal(raw, &st); err != nil {
		return core.State{}, fmt.Errorf("state unmarshal: %w", err)
	}
	return st, nil
}

// List returns the run IDs of every persisted state.
func (s *FileStateStore) List(_ context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.BaseDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		out = append(out, name[:len(name)-len(".json")])
	}
	return out, nil
}

// Delete removes a persisted state.
func (s *FileStateStore) Delete(_ context.Context, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(runID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// FileWAL appends one Input per line as JSON.
type FileWAL struct {
	BaseDir string
	mu      sync.Mutex
}

// NewFileWAL returns a WAL rooted at baseDir.
func NewFileWAL(baseDir string) (*FileWAL, error) {
	full := filepath.Join(baseDir, "wal")
	if err := os.MkdirAll(full, 0o750); err != nil {
		return nil, fmt.Errorf("filestore: mkdir wal: %w", err)
	}
	return &FileWAL{BaseDir: full}, nil
}

func (w *FileWAL) path(runID string) string {
	return filepath.Join(w.BaseDir, runID+".jsonl")
}

// Append writes one Input as a single JSON line. Order matches arrival order.
func (w *FileWAL) Append(_ context.Context, runID string, _ int, in core.Input) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	raw, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("wal marshal: %w", err)
	}
	f, err := os.OpenFile(w.path(runID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("wal open: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("wal write: %w", err)
	}
	return nil
}

// Replay reads every Input whose Seq > sinceSeq, in write order.
// Items without an explicit Seq are assigned the line index, so older
// checkpoints that pre-date the Seq field still load.
func (w *FileWAL) Replay(_ context.Context, runID string, sinceSeq int) ([]core.Input, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	raw, err := os.ReadFile(w.path(runID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	trimmed := raw
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	var inputs []core.Input
	lineIdx := 0
	for len(trimmed) > 0 {
		// find next newline
		end := -1
		for i, c := range trimmed {
			if c == '\n' {
				end = i
				break
			}
		}
		if end < 0 {
			end = len(trimmed)
		}
		line := trimmed[:end]
		if len(line) > 0 {
			var in core.Input
			if err := json.Unmarshal(line, &in); err != nil {
				return nil, fmt.Errorf("wal decode line %d: %w", lineIdx, err)
			}
			// Choose Seq-aware comparison; fall back to line index.
			if in.Seq > sinceSeq {
				inputs = append(inputs, in)
			}
			lineIdx++
		}
		if end >= len(trimmed) {
			break
		}
		trimmed = trimmed[end+1:]
	}
	return inputs, nil
}

// Truncate drops Inputs whose Seq <= uptoSeq. Used after a successful
// compaction so the WAL does not grow unbounded.
func (w *FileWAL) Truncate(_ context.Context, runID string, uptoSeq int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	raw, err := os.ReadFile(w.path(runID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Find the byte offset just past the last contiguous line whose
	// Seq <= uptoSeq. Lines with Seq > uptoSeq are kept and stop the scan.
	pos := 0
	for pos < len(raw) {
		nl := -1
		for i := pos; i < len(raw); i++ {
			if raw[i] == '\n' {
				nl = i
				break
			}
		}
		endLine := len(raw)
		if nl >= 0 {
			endLine = nl
		}
		var in core.Input
		if err := json.Unmarshal(raw[pos:endLine], &in); err == nil && in.Seq <= uptoSeq {
			pos = endLine + 1
			if nl < 0 {
				// last line, no trailing newline
				pos = len(raw)
			}
			continue
		}
		break
	}
	if pos >= len(raw) {
		return os.Remove(w.path(runID))
	}
	return os.WriteFile(w.path(runID), raw[pos:], 0o640)
}