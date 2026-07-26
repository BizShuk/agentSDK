package source_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/prompt/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sectionsOfReminder(t *testing.T, s prompt.Source, req prompt.Req) []prompt.Section {
	t.Helper()
	got, err := s.Sections(context.Background(), req)
	require.NoError(t, err)
	return got
}

func TestReminderSourceOnlySpeaksNearTheBudget(t *testing.T) {
	cases := []struct {
		name     string
		turn     int
		maxTurns int
		want     bool
	}{
		{"plenty left", 1, 20, false},
		{"getting close", 17, 20, true},
		{"last turn", 19, 20, true},
		{"no budget set", 5, 0, false},
		{"already over", 25, 20, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sectionsOfReminder(t, source.ReminderSource(), prompt.Req{
				State: core.State{Turn: tc.turn, Budget: core.Budget{MaxTurns: tc.maxTurns}},
			})
			if !tc.want {
				assert.Empty(t, got, "a reminder on every turn is noise the model learns to ignore")
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, prompt.SLOT_REMINDER, got[0].Slot,
				"reminders ride with the user message, never rewrite the system prompt")
			assert.Contains(t, got[0].Text, "remaining")
		})
	}
}
