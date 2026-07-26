package core_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateClone(t *testing.T) {
	s := core.State{
		RunID: "run-1",
		Messages: []core.Message{
			{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}},
		},
		WorkingMemory:    map[string]any{"k": "v"},
		PendingApprovals: []core.PendingApproval{{ID: "a1"}},
	}

	clone := s.Clone()
	assert.Equal(t, s.RunID, clone.RunID)
	assert.Len(t, clone.Messages, 1)
	assert.Equal(t, "v", clone.WorkingMemory["k"])

	// mutate clone, original must remain
	clone.Messages[0].Parts[0].Text = "bye"
	clone.WorkingMemory["k"] = "modified"
	assert.Equal(t, "hi", s.Messages[0].Parts[0].Text)
	assert.Equal(t, "v", s.WorkingMemory["k"])
}

func TestStateJSONRoundTrip(t *testing.T) {
	in := core.State{
		RunID:          "run-xyz",
		Turn:           3,
		Autonomy:       core.AUTONOMY_L2,
		ReasoningStyle: core.REASON_REACT,
		Messages: []core.Message{
			{Role: core.ROLE_USER, Ts: time.Unix(1700000000, 0).UTC(),
				Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hello"}}},
		},
		PendingApprovals: []core.PendingApproval{{
			ID: "apr-1", Risk: core.RISK_LEVEL_HIGH,
			Summary: "delete prod",
		}},
		Status:    core.RUN_STATUS_RUNNING,
		UpdatedAt: time.Unix(1700000001, 0).UTC(),
		Budget:    core.Budget{MaxTurns: 10, UsedTurns: 3, StartedAt: time.Unix(1700000000, 0).UTC()},
	}

	raw, err := json.Marshal(in)
	require.NoError(t, err)

	var out core.State
	require.NoError(t, json.Unmarshal(raw, &out))

	assert.Equal(t, in.RunID, out.RunID)
	assert.Equal(t, in.Turn, out.Turn)
	assert.Equal(t, in.Autonomy, out.Autonomy)
	assert.Equal(t, in.ReasoningStyle, out.ReasoningStyle)
	require.Len(t, out.Messages, 1)
	assert.Equal(t, "hello", out.Messages[0].Parts[0].Text)
	require.Len(t, out.PendingApprovals, 1)
	assert.Equal(t, core.RISK_LEVEL_HIGH, out.PendingApprovals[0].Risk)
	assert.Equal(t, core.RUN_STATUS_RUNNING, out.Status)
}

func TestStateJSONWireStrings(t *testing.T) {
	s := core.State{
		RunID:          "r1",
		ReasoningStyle: core.REASON_REACT,
		Messages: []core.Message{{
			Role:  core.ROLE_USER,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}},
		}},
		WorkingMemory: map[string]any{"k": "v"},
		Status:        core.RUN_STATUS_RUNNING,
		Autonomy:      core.AUTONOMY_L2,
	}
	raw, err := json.Marshal(s)
	require.NoError(t, err)
	body := string(raw)
	// Plain English wire strings:
	assert.Contains(t, body, `"thinking_kind":"think_then_act"`)
	assert.Contains(t, body, `"kind":"plain_text"`)
	// WorkingMemory still uses the legacy JSON tag "scratch":
	assert.Contains(t, body, `"scratch":{"k":"v"}`)
	// Old academic wire strings gone:
	assert.NotContains(t, body, `"react"`)
	assert.NotContains(t, body, `"percept"`)
	// Old "text" PartKind value is gone (replaced by "plain_text" — the
	// word "text" itself is fine because Part.Text is still a "text" field).
	assert.NotContains(t, body, `"kind":"text"`)
}
