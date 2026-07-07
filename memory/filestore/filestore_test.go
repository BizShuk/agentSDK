package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMigratesOldWireStrings(t *testing.T) {
	dir := t.TempDir()
	old := map[string]any{
		"run_id":        "r-old",
		"turn":          3,
		"autonomy":      2,
		"thinking_kind": "react",
		"messages": []map[string]any{{
			"role": "user",
			"chunks": []map[string]any{{
				"kind": "text",
				"text": "hello",
			}},
		}},
		"scratch":        map[string]any{"k": "v"},
		"status":         "running",
		"last_input_seq": 1,
	}
	raw, _ := json.Marshal(old)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "states"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "states", "r-old.json"), raw, 0o640))

	s, err := NewJSONFileStateStore(dir)
	require.NoError(t, err)
	loaded, err := s.Load(context.Background(), "r-old")
	require.NoError(t, err)
	assert.Equal(t, core.REASON_REACT, loaded.ReasoningStyle)
	require.Len(t, loaded.Messages, 1)
	require.Len(t, loaded.Messages[0].Parts, 1)
	assert.Equal(t, core.PART_KIND_PLAIN_TEXT, loaded.Messages[0].Parts[0].Kind)
	assert.Equal(t, "v", loaded.WorkingMemory["k"])
}

func TestJSONLFileLogRoundTrip(t *testing.T) {
	dir := t.TempDir()
	wal, err := NewJSONLFileLog(dir)
	require.NoError(t, err)

	ev1 := core.Event{Kind: core.EVENT_OBSERVATION, Seq: 1, Observation: &core.Observation{ID: "o1", Payload: "hi"}}
	ev2 := core.Event{Kind: core.EVENT_TOOL_RESULT, Seq: 2, ToolResult: &core.ToolResult{CallID: "c1", Name: "x", OK: true}}
	require.NoError(t, wal.Append(context.Background(), "r1", 1, ev1))
	require.NoError(t, wal.Append(context.Background(), "r1", 2, ev2))

	all, err := wal.Read(context.Background(), "r1", 0)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, core.EVENT_OBSERVATION, all[0].Kind)
	assert.Equal(t, core.EVENT_TOOL_RESULT, all[1].Kind)

	// Read with sinceSeq=1 skips first event.
	since1, err := wal.Read(context.Background(), "r1", 1)
	require.NoError(t, err)
	require.Len(t, since1, 1)
	assert.Equal(t, 2, since1[0].Seq)
}
