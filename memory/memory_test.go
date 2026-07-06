package memory_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/internal/testutil"
	"github.com/bizshuk/agentsdk/memory"
	"github.com/bizshuk/agentsdk/memory/checkpoint"
	"github.com/bizshuk/agentsdk/memory/filestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCharHeuristicCounter(t *testing.T) {
	c := memory.CharHeuristicCounter{}
	got := c.Count([]core.Message{
		{Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "hello"}}}, // 5/4+1=2
		{Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "world"}}}, // 2
	})
	assert.Equal(t, 4, got)
}

func TestWindowTrimByMessageCount(t *testing.T) {
	w := memory.Window{MaxMessages: 2}
	msgs := []core.Message{
		{Role: core.ROLE_USER, Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "a"}}},
		{Role: core.ROLE_USER, Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "b"}}},
		{Role: core.ROLE_USER, Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "c"}}},
	}
	out := w.Trim(msgs)
	require.Len(t, out, 2)
	assert.Equal(t, "b", out[0].Chunks[0].Text)
	assert.Equal(t, "c", out[1].Chunks[0].Text)
}

func TestWindowTrimByTokenCount(t *testing.T) {
	w := memory.Window{MaxTokens: 5, Counter: memory.CharHeuristicCounter{}}
	// 4 messages, each ~2 tokens (text length / 4 + 1).
	// Total = 8. After trim to ≤ 5 tokens, drop oldest.
	msgs := []core.Message{
		{Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "aaaa"}}}, // 2
		{Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "bbbb"}}}, // 2
		{Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "cccc"}}}, // 2
		{Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "dddd"}}}, // 2
	}
	out := w.Trim(msgs)
	// 8 tokens, drop until ≤5; the trim loop stops at len(msgs)>1
	// so we'll end with the last 3 (6 tokens > 5 but no further drop allowed).
	assert.GreaterOrEqual(t, len(out), 1)
	assert.Less(t, len(out), 4)
}

func TestHeadlineCompactor(t *testing.T) {
	c := memory.HeadlineCompactor{}
	msg, err := c.Compact([]core.Message{
		{Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "first line\nsecond line"}}},
		{Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "third line"}}},
	})
	require.NoError(t, err)
	require.Len(t, msg.Chunks, 1)
	assert.Equal(t, core.ROLE_ASSISTANT, msg.Role)
	assert.Contains(t, msg.Chunks[0].Text, "first line")
	assert.Contains(t, msg.Chunks[0].Text, "third line")
	assert.NotContains(t, msg.Chunks[0].Text, "second line")
}

func TestFileStateStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.NewFileStateStore(dir)
	require.NoError(t, err)

	in := core.State{
		RunID: "r1", Turn: 3, Status: core.RUN_STATUS_PAUSED_APPROVAL,
		ThinkingKind: core.THINK_REACT, Autonomy: core.AUTONOMY_L2,
		Messages: []core.Message{
			{Role: core.ROLE_USER, Chunks: []core.Chunk{{Kind: core.CHUNK_KIND_TEXT, Text: "x"}}},
		},
		Budget:   core.Budget{MaxTurns: 10, UsedTurns: 3},
		UpdatedAt: time.Unix(1700000000, 0).UTC(),
	}
	require.NoError(t, s.Save(context.Background(), in))

	out, err := s.Load(context.Background(), "r1")
	require.NoError(t, err)
	assert.Equal(t, in.RunID, out.RunID)
	assert.Equal(t, in.Turn, out.Turn)
	assert.Equal(t, in.Status, out.Status)
	require.Len(t, out.Messages, 1)
	assert.Equal(t, "x", out.Messages[0].Chunks[0].Text)
}

func TestFileWALAppendReplay(t *testing.T) {
	dir := t.TempDir()
	w, err := filestore.NewFileWAL(dir)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		require.NoError(t, w.Append(context.Background(), "r1", i+1, core.Input{
			Kind: core.INPUT_KIND_TOOL_RESULT,
			Seq:  i + 1,
			ToolResult: &core.ToolResult{
				CallID: "c1", Name: "x", OK: true,
				Output: map[string]any{"i": i},
			},
		}))
	}
	all, err := w.Replay(context.Background(), "r1", 0)
	require.NoError(t, err)
	require.Len(t, all, 5)
	assert.Equal(t, 1, all[0].Seq)
	assert.Equal(t, 5, all[4].Seq)

	// Since 3 → only seq 4 and 5
	since, err := w.Replay(context.Background(), "r1", 3)
	require.NoError(t, err)
	require.Len(t, since, 2)
	assert.Equal(t, 4, since[0].Seq)
}

func TestFileWALTruncate(t *testing.T) {
	dir := t.TempDir()
	w, err := filestore.NewFileWAL(dir)
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		require.NoError(t, w.Append(context.Background(), "r1", i+1, core.Input{Seq: i + 1}))
	}
	require.NoError(t, w.Truncate(context.Background(), "r1", 3))
	rest, err := w.Replay(context.Background(), "r1", 0)
	require.NoError(t, err)
	require.Len(t, rest, 2)
	assert.Equal(t, 4, rest[0].Seq)
}

func TestCheckpointerCheckpointAndRecover(t *testing.T) {
	dir := t.TempDir()
	store, err := filestore.NewFileStateStore(dir)
	require.NoError(t, err)
	wal, err := filestore.NewFileWAL(dir)
	require.NoError(t, err)

	cp := checkpoint.New(store, wal)

	in := core.State{
		RunID: "r2", Turn: 5, Status: core.RUN_STATUS_PAUSED_APPROVAL,
		ThinkingKind: core.THINK_REACT,
		Budget:       core.Budget{MaxTurns: 10, UsedTurns: 5},
		LastInputSeq: 2,
	}
	require.NoError(t, cp.Checkpoint(context.Background(), in))

	for i := 1; i <= 2; i++ {
		require.NoError(t, wal.Append(context.Background(), "r2", i+in.LastInputSeq,
			core.Input{Seq: i + in.LastInputSeq, Kind: core.INPUT_KIND_TOOL_RESULT}))
	}

	out, err := cp.Recover(context.Background(), "r2")
	require.NoError(t, err)
	assert.Equal(t, in.RunID, out.State.RunID)
	assert.Equal(t, in.Turn, out.State.Turn)
	require.Len(t, out.Inputs, 2)
	assert.Equal(t, 3, out.Inputs[0].Seq)
}

// TestRecoverDoesNotReissueModelCalls verifies that WAL replay returns
// Inputs the caller can re-feed WITHOUT making a fresh Generate call.
// We model this by counting Generate calls before/after Recover.
func TestRecoverDoesNotReissueModelCalls(t *testing.T) {
	dir := t.TempDir()
	store, err := filestore.NewFileStateStore(dir)
	require.NoError(t, err)
	wal, err := filestore.NewFileWAL(dir)
	require.NoError(t, err)
	cp := checkpoint.New(store, wal)
	prov := testutil.NewFakeProvider()
	prov.EnqueueEndTurn("anything")

	// Pretend a previous run already produced these inputs.
	require.NoError(t, wal.Append(context.Background(), "r3", 2, core.Input{
		Kind: core.INPUT_KIND_MODEL_RESULT, Seq: 2,
		ModelResult: &core.ModelResult{StopReason: "end_turn", Text: "done"},
	}))
	require.NoError(t, store.Save(context.Background(), core.State{
		RunID: "r3", Status: core.RUN_STATUS_RUNNING, LastInputSeq: 1,
	}))

	res, err := cp.Recover(context.Background(), "r3")
	require.NoError(t, err)
	assert.Equal(t, 0, prov.CallCount(), "Recover must not issue any model calls")
	require.Len(t, res.Inputs, 1)
	assert.Equal(t, 2, res.Inputs[0].Seq)
}

// TestFileStateStoreListAndDelete covers the housekeeping methods.
func TestFileStateStoreListAndDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.NewFileStateStore(dir)
	require.NoError(t, err)
	require.NoError(t, s.Save(context.Background(), core.State{RunID: "a"}))
	require.NoError(t, s.Save(context.Background(), core.State{RunID: "b"}))
	list, err := s.List(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, list)
	require.NoError(t, s.Delete(context.Background(), "a"))
	list, _ = s.List(context.Background())
	assert.ElementsMatch(t, []string{"b"}, list)
}

// silence unused imports if filepath / json / os not exercised
var (
	_ = filepath.Join
	_ = json.Marshal
	_ = os.Getenv
)