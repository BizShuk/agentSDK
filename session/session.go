// Package session is the management layer above memory's StateStore and
// WriteAheadLog: session metadata, listing by working directory, resume
// helpers, forking, and the parent tree. The WAL JSONL stays the single
// source of truth for a transcript (the pattern shared by claude-code,
// codex, and pi); this package only adds naming and lineage on top.
//
// The package depends on core ports only — inject memory/filestore (or any
// other implementation) at the composition root.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// Meta is the per-session sidecar record, persisted as Dir/<id>.json.
type Meta struct {
	ID        string    `json:"id"`
	Parent    string    `json:"parent,omitempty"` // fork lineage
	Title     string    `json:"title,omitempty"`
	Cwd       string    `json:"cwd,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Node is one session in the fork tree.
type Node struct {
	Meta     Meta
	Children []*Node
}

// Manager binds the metadata dir to the state/WAL ports.
type Manager struct {
	Store core.StateStore
	Log   core.WriteAheadLog
	Dir   string // metadata directory, e.g. <appdata>/sessions

	Now   func() time.Time // nil → time.Now
	NewID func() string    // nil → timestamp-derived id
}

// NewManager creates the metadata dir and returns a Manager.
func NewManager(store core.StateStore, log core.WriteAheadLog, dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("session: mkdir: %w", err)
	}
	return &Manager{Store: store, Log: log, Dir: dir}, nil
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) newID() string {
	if m.NewID != nil {
		return m.NewID()
	}
	t := m.now().UTC()
	return fmt.Sprintf("s%s-%06d", t.Format("20060102-150405"), t.Nanosecond()/1000)
}

func (m *Manager) metaPath(id string) string {
	return filepath.Join(m.Dir, id+".json")
}

// Save persists one Meta atomically (write-temp + rename).
func (m *Manager) Save(meta Meta) error {
	if meta.ID == "" {
		return errors.New("session: meta.ID required")
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("session: meta marshal: %w", err)
	}
	tmp := m.metaPath(meta.ID) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o640); err != nil {
		return fmt.Errorf("session: meta write: %w", err)
	}
	return os.Rename(tmp, m.metaPath(meta.ID))
}

// Get loads one Meta by session ID.
func (m *Manager) Get(id string) (Meta, error) {
	raw, err := os.ReadFile(m.metaPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return Meta{}, fmt.Errorf("session not found: %s", id)
		}
		return Meta{}, err
	}
	var meta Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return Meta{}, fmt.Errorf("session: meta decode %s: %w", id, err)
	}
	return meta, nil
}

// List returns every session, newest first. A non-empty cwd filters to
// sessions started in that directory — the per-project session list every
// surveyed client ships.
func (m *Manager) List(cwd string) ([]Meta, error) {
	entries, err := os.ReadDir(m.Dir)
	if err != nil {
		return nil, err
	}
	out := make([]Meta, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		meta, err := m.Get(strings.TrimSuffix(name, ".json"))
		if err != nil {
			continue // skip unreadable sidecars; they never hide other sessions
		}
		if cwd != "" && meta.Cwd != cwd {
			continue
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Latest returns the newest session for cwd ("" = any) — the `--continue`
// behavior.
func (m *Manager) Latest(cwd string) (Meta, error) {
	all, err := m.List(cwd)
	if err != nil {
		return Meta{}, err
	}
	if len(all) == 0 {
		return Meta{}, errors.New("session: none found")
	}
	return all[0], nil
}

// Begin records a new session's metadata for runID.
func (m *Manager) Begin(runID, title, cwd string) (Meta, error) {
	meta := Meta{ID: runID, Title: title, Cwd: cwd, CreatedAt: m.now().UTC()}
	if err := m.Save(meta); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

// Fork copies session id's state snapshot and WAL into a new session and
// records the parent link. The original is left untouched — in-place tree
// branching à la pi's /tree is layered on this same lineage data.
func (m *Manager) Fork(ctx context.Context, id, title string) (Meta, error) {
	if m.Store == nil {
		return Meta{}, errors.New("session: Fork requires Store")
	}
	st, err := m.Store.Load(ctx, id)
	if err != nil {
		return Meta{}, fmt.Errorf("session fork: %w", err)
	}
	newID := m.newID()
	st.RunID = newID
	if err := m.Store.Save(ctx, st); err != nil {
		return Meta{}, fmt.Errorf("session fork save: %w", err)
	}
	if m.Log != nil {
		events, err := m.Log.Read(ctx, id, -1) // -1: include Seq 0 entries
		if err != nil {
			return Meta{}, fmt.Errorf("session fork wal read: %w", err)
		}
		for _, ev := range events {
			if err := m.Log.Append(ctx, newID, ev.Seq, ev); err != nil {
				return Meta{}, fmt.Errorf("session fork wal append: %w", err)
			}
		}
	}
	parent, err := m.Get(id)
	if err != nil {
		// Forking an unnamed run is allowed — no sidecar means no cwd to inherit.
		parent = Meta{}
	}
	meta := Meta{ID: newID, Parent: id, Title: title, Cwd: parent.Cwd, CreatedAt: m.now().UTC()}
	if err := m.Save(meta); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

// Tree builds the fork forest for cwd ("" = all sessions): roots are
// sessions whose parent is absent from the listing. Children are ordered
// oldest first; roots newest first (matching List).
func (m *Manager) Tree(cwd string) ([]*Node, error) {
	all, err := m.List(cwd)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]*Node, len(all))
	for _, meta := range all {
		nodes[meta.ID] = &Node{Meta: meta}
	}
	var roots []*Node
	for _, meta := range all {
		n := nodes[meta.ID]
		if meta.Parent != "" {
			if p, ok := nodes[meta.Parent]; ok {
				p.Children = append(p.Children, n)
				continue
			}
		}
		roots = append(roots, n)
	}
	for _, n := range nodes {
		sort.Slice(n.Children, func(i, j int) bool {
			return n.Children[i].Meta.CreatedAt.Before(n.Children[j].Meta.CreatedAt)
		})
	}
	return roots, nil
}
