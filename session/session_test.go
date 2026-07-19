package session

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore / fakeLog keep session decoupled from memory/filestore in tests.
type fakeStore struct{ states map[string]core.State }

func newFakeStore() *fakeStore { return &fakeStore{states: map[string]core.State{}} }

func (f *fakeStore) Save(_ context.Context, s core.State) error { f.states[s.RunID] = s; return nil }
func (f *fakeStore) Load(_ context.Context, runID string) (core.State, error) {
	s, ok := f.states[runID]
	if !ok {
		return core.State{}, fmt.Errorf("run not found: %s", runID)
	}
	return s, nil
}
func (f *fakeStore) List(_ context.Context) ([]string, error)  { return nil, nil }
func (f *fakeStore) Delete(_ context.Context, id string) error { delete(f.states, id); return nil }

type fakeLog struct{ events map[string][]core.Event }

func newFakeLog() *fakeLog { return &fakeLog{events: map[string][]core.Event{}} }

func (f *fakeLog) Append(_ context.Context, runID string, _ int, ev core.Event) error {
	f.events[runID] = append(f.events[runID], ev)
	return nil
}
func (f *fakeLog) Read(_ context.Context, runID string, sinceSeq int) ([]core.Event, error) {
	var out []core.Event
	for _, ev := range f.events[runID] {
		if ev.Seq > sinceSeq {
			out = append(out, ev)
		}
	}
	return out, nil
}
func (f *fakeLog) TruncateFrom(_ context.Context, _ string, _ int) error { return nil }

func newTestManager(t *testing.T) (*Manager, *fakeStore, *fakeLog) {
	t.Helper()
	store, log := newFakeStore(), newFakeLog()
	m, err := NewManager(store, log, t.TempDir())
	require.NoError(t, err)
	base := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	tick := 0
	m.Now = func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Minute) }
	n := 0
	m.NewID = func() string { n++; return fmt.Sprintf("fork-%d", n) }
	return m, store, log
}

func TestListFilterAndOrder(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.Begin("run-a", "first", "/proj/x")
	require.NoError(t, err)
	_, err = m.Begin("run-b", "second", "/proj/y")
	require.NoError(t, err)
	_, err = m.Begin("run-c", "third", "/proj/x")
	require.NoError(t, err)

	all, err := m.List("")
	require.NoError(t, err)
	assert.Len(t, all, 3)
	assert.Equal(t, "run-c", all[0].ID, "newest first")

	filtered, err := m.List("/proj/x")
	require.NoError(t, err)
	assert.Len(t, filtered, 2)

	latest, err := m.Latest("/proj/x")
	require.NoError(t, err)
	assert.Equal(t, "run-c", latest.ID)
}

func TestForkCopiesStateAndWAL(t *testing.T) {
	m, store, log := newTestManager(t)
	ctx := context.Background()

	_, err := m.Begin("run-a", "origin", "/proj/x")
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, core.State{RunID: "run-a", Turn: 3}))
	require.NoError(t, log.Append(ctx, "run-a", 1, core.Event{Kind: core.EVENT_MODEL_REPLY, Seq: 1}))
	require.NoError(t, log.Append(ctx, "run-a", 2, core.Event{Kind: core.EVENT_TOOL_RESULT, Seq: 2}))

	meta, err := m.Fork(ctx, "run-a", "branch")
	require.NoError(t, err)
	assert.Equal(t, "fork-1", meta.ID)
	assert.Equal(t, "run-a", meta.Parent)
	assert.Equal(t, "/proj/x", meta.Cwd, "cwd inherited from parent")

	forked, err := store.Load(ctx, "fork-1")
	require.NoError(t, err)
	assert.Equal(t, 3, forked.Turn)
	assert.Equal(t, "fork-1", forked.RunID, "RunID rewritten")

	events, err := log.Read(ctx, "fork-1", -1)
	require.NoError(t, err)
	assert.Len(t, events, 2, "WAL copied")

	orig, err := store.Load(ctx, "run-a")
	require.NoError(t, err)
	assert.Equal(t, "run-a", orig.RunID, "original untouched")
}

func TestTreeLineage(t *testing.T) {
	m, store, _ := newTestManager(t)
	ctx := context.Background()

	_, err := m.Begin("root-1", "r", "/p")
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, core.State{RunID: "root-1"}))

	child1, err := m.Fork(ctx, "root-1", "c1")
	require.NoError(t, err)
	_, err = m.Fork(ctx, "root-1", "c2")
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, core.State{RunID: child1.ID}))
	_, err = m.Fork(ctx, child1.ID, "grandchild")
	require.NoError(t, err)

	roots, err := m.Tree("/p")
	require.NoError(t, err)
	require.Len(t, roots, 1)
	root := roots[0]
	assert.Equal(t, "root-1", root.Meta.ID)
	require.Len(t, root.Children, 2)
	assert.Equal(t, "fork-1", root.Children[0].Meta.ID, "children oldest first")
	require.Len(t, root.Children[0].Children, 1)
	assert.Equal(t, "grandchild", root.Children[0].Children[0].Meta.Title)
}
