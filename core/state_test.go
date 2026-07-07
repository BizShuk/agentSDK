package core_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudgetExceeded(t *testing.T) {
	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	tests := []struct {
		name     string
		budget   core.Budget
		wantExcd bool
		wantWhy  string
	}{
		{
			name:     "empty budget never exceeds",
			budget:   core.Budget{NowFunc: clock},
			wantExcd: false,
		},
		{
			name: "turn budget hit",
			budget: core.Budget{
				MaxTurns: 3, UsedTurns: 3,
				NowFunc: clock,
			},
			wantExcd: true, wantWhy: "turn_budget",
		},
		{
			name: "token budget hit",
			budget: core.Budget{
				MaxTokens: 100, UsedTokens: 200,
				NowFunc: clock,
			},
			wantExcd: true, wantWhy: "token_budget",
		},
		{
			name: "wall time exceeded",
			budget: core.Budget{
				MaxWallTime: 10 * time.Minute,
				StartedAt:   now.Add(-11 * time.Minute),
				NowFunc:     clock,
			},
			wantExcd: true, wantWhy: "wall_time_budget",
		},
		{
			name: "wall time within budget",
			budget: core.Budget{
				MaxWallTime: 10 * time.Minute,
				StartedAt:   now.Add(-5 * time.Minute),
				NowFunc:     clock,
			},
			wantExcd: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			excd, why := tc.budget.Exceeded()
			assert.Equal(t, tc.wantExcd, excd)
			if tc.wantWhy != "" {
				assert.Equal(t, tc.wantWhy, why)
			}
		})
	}
}

func TestStateClone(t *testing.T) {
	s := core.State{
		RunID: "run-1",
		Messages: []core.Message{
			{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hi"}}},
		},
		WorkingMemory: map[string]any{"k": "v"},
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

func TestRunStatusTerminal(t *testing.T) {
	assert.False(t, core.RUN_STATUS_RUNNING.Terminal())
	assert.False(t, core.RUN_STATUS_PAUSED_APPROVAL.Terminal())
	assert.True(t, core.RUN_STATUS_COMPLETED.Terminal())
	assert.True(t, core.RUN_STATUS_FAILED.Terminal())
	assert.True(t, core.RUN_STATUS_ABORTED.Terminal())
}

func TestAutonomyString(t *testing.T) {
	assert.Equal(t, "L0", core.AUTONOMY_L0.String())
	assert.Equal(t, "L2", core.AUTONOMY_L2.String())
	assert.Equal(t, "L4", core.AUTONOMY_L4.String())
	assert.Equal(t, "L?", core.AutonomyLevel(99).String())
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
