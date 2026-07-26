package source

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/prompt"
)

// ReminderSource re-states the run's remaining budget each turn.
//
// This is the seam the design leaves open for "remind the model of the
// last response" or "restate the outstanding TODOs": a reminder reads
// Req.State and contributes to SLOT_REMINDER, which rides with the user
// message. It never rewrites the system prompt — doing so would break the
// cached prefix every turn, and trimming history stays memory's job.
func ReminderSource() prompt.Source {
	return prompt.SourceFunc(func(_ context.Context, req prompt.Req) ([]prompt.Section, error) {
		max := req.State.Budget.MaxTurns
		if max <= 0 {
			return nil, nil
		}
		left := max - req.State.Turn
		if left > 3 || left < 0 {
			// Only worth saying when it starts to matter. A reminder on
			// every turn is noise the model learns to ignore.
			return nil, nil
		}
		return []prompt.Section{{
			Slot:  prompt.SLOT_REMINDER,
			Name:  "budget",
			Text:  fmt.Sprintf("<budget>%d of %d turns remaining — wrap up.</budget>", left, max),
			Order: prompt.ORDER_REMINDER,
		}}, nil
	})
}
