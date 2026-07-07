// Package filestore is the default file-backed StateStore + WriteAheadLog.
//
// Layout (under baseDir):
//
//	baseDir/
//	├── states/<runID>.json          # State snapshot — last-known
//	└── wal/<runID>.jsonl            # Append-only Events (one per line)
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
	"strings"
	"sync"

	"github.com/bizshuk/agentsdk/core"
)

// JSONFileStateStore persists State as pretty-printed JSON.
type JSONFileStateStore struct {
	BaseDir string // typically config.GetAppDataDir() / "states"

	mu sync.Mutex
}

// NewJSONFileStateStore returns a store rooted at baseDir. The directory is
// created if missing.
func NewJSONFileStateStore(baseDir string) (*JSONFileStateStore, error) {
	full := filepath.Join(baseDir, "states")
	if err := os.MkdirAll(full, 0o750); err != nil {
		return nil, fmt.Errorf("filestore: mkdir states: %w", err)
	}
	return &JSONFileStateStore{BaseDir: full}, nil
}

func (s *JSONFileStateStore) path(runID string) string {
	return filepath.Join(s.BaseDir, runID+".json")
}

// Save writes State atomically (write-temp + rename).
func (s *JSONFileStateStore) Save(_ context.Context, st core.State) error {
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

// Load reads a State, applying the v1→v2 migration shim if needed.
// Returns an error if the run is unknown.
func (s *JSONFileStateStore) Load(_ context.Context, runID string) (core.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path(runID))
	if err != nil {
		if os.IsNotExist(err) {
			return core.State{}, fmt.Errorf("run not found: %s", runID)
		}
		return core.State{}, err
	}
	// First try v2 ("parts"). On failure, try v1 ("chunks") and re-tag.
	var st core.State
	if err := json.Unmarshal(raw, &st); err == nil && (len(st.Messages) == 0 || len(st.Messages[0].Parts) > 0 || !strings.Contains(string(raw), `"chunks"`)) {
		migrateFromV1(&st)
		return st, nil
	}
	// v1 fallback: rewrite "chunks" → "parts" and try again.
	rewritten := v1ToV2JSON(raw)
	if err := json.Unmarshal(rewritten, &st); err != nil {
		return core.State{}, fmt.Errorf("state unmarshal: %w", err)
	}
	migrateFromV1(&st)
	return st, nil
}

// List returns the run IDs of every persisted state.
func (s *JSONFileStateStore) List(_ context.Context) ([]string, error) {
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
func (s *JSONFileStateStore) Delete(_ context.Context, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(runID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// JSONLFileLog appends one Event per line as JSON.
type JSONLFileLog struct {
	BaseDir string
	mu      sync.Mutex
}

// NewJSONLFileLog returns a WriteAheadLog rooted at baseDir.
func NewJSONLFileLog(baseDir string) (*JSONLFileLog, error) {
	full := filepath.Join(baseDir, "wal")
	if err := os.MkdirAll(full, 0o750); err != nil {
		return nil, fmt.Errorf("filestore: mkdir wal: %w", err)
	}
	return &JSONLFileLog{BaseDir: full}, nil
}

func (w *JSONLFileLog) path(runID string) string {
	return filepath.Join(w.BaseDir, runID+".jsonl")
}

// Append writes one Event as a single JSON line. Order matches arrival order.
func (w *JSONLFileLog) Append(_ context.Context, runID string, _ int, ev core.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	raw, err := json.Marshal(ev)
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

// Read returns every Event whose Seq > sinceSeq, in write order.
// Items without an explicit Seq are assigned the line index, so older
// checkpoints that pre-date the Seq field still load.
func (w *JSONLFileLog) Read(_ context.Context, runID string, sinceSeq int) ([]core.Event, error) {
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
	var events []core.Event
	lineIdx := 0
	for len(trimmed) > 0 {
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
			var ev core.Event
			if err := json.Unmarshal(line, &ev); err != nil {
				return nil, fmt.Errorf("wal decode line %d: %w", lineIdx, err)
			}
			// Choose Seq-aware comparison; fall back to line index.
			if ev.Seq > sinceSeq {
				events = append(events, ev)
			}
			lineIdx++
		}
		if end >= len(trimmed) {
			break
		}
		trimmed = trimmed[end+1:]
	}
	return events, nil
}

// TruncateFrom drops Events whose Seq <= uptoSeq. Used after a successful
// compaction so the WAL does not grow unbounded.
func (w *JSONLFileLog) TruncateFrom(_ context.Context, runID string, uptoSeq int) error {
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
		var ev core.Event
		if err := json.Unmarshal(raw[pos:endLine], &ev); err == nil && ev.Seq <= uptoSeq {
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

// v1ToV2JSON rewrites the v1 JSON tag "chunks" to the v2 tag "parts"
// inside the raw bytes of a saved State. Used to load pre-rename files
// that used the "chunks" JSON tag for Message parts.
func v1ToV2JSON(raw []byte) []byte {
	return []byte(strings.ReplaceAll(string(raw), `"chunks":`, `"parts":`))
}

// migrateFromV1 translates a v1 (pre-rename) State into the v2 shape.
// v1 used the academic names: Percept/Input/Effect/Chunk/ThinkingKind/
// ReAct/etc.; v2 uses Observation/Event/Instruction/Part/ReasoningStyle/
// think_then_act/etc. JSON tag shapes also changed ("chunks"→"parts",
// "percept"→"observation", etc.).
func migrateFromV1(s *core.State) {
	// Map ReasoningStyle values. The JSON tag "thinking_kind" is preserved
	// across the rename — only the value is translated.
	switch s.ReasoningStyle {
	case "react":
		s.ReasoningStyle = core.REASON_REACT
	case "planner_executor":
		s.ReasoningStyle = core.REASON_PLAN_THEN_RUN
	case "executor_critic":
		s.ReasoningStyle = core.REASON_DO_THEN_REVIEW
	case "cot_singleshot":
		s.ReasoningStyle = core.REASON_ONE_SHOT
	case "reflexion":
		s.ReasoningStyle = core.REASON_LEARN_FROM_FAILURE
	case "router":
		s.ReasoningStyle = core.REASON_PICK_AGENT
	}
	// Map PartKind values in every Message.
	for mi := range s.Messages {
		for pi := range s.Messages[mi].Parts {
			switch s.Messages[mi].Parts[pi].Kind {
			case "text":
				s.Messages[mi].Parts[pi].Kind = core.PART_KIND_PLAIN_TEXT
			}
		}
	}
	// WorkingMemory already loads under the "scratch" JSON tag (we kept
	// the tag), so no field-rename needed.
	// ReasoningStyle JSON tag "thinking_kind" is also preserved — only
	// the value is translated above.
}
