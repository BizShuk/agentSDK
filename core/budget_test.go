package core_test

import (
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exceeded, reason := test.budget.Exceeded()
			assert.Equal(t, test.wantExcd, exceeded)
			if test.wantWhy != "" {
				assert.Equal(t, test.wantWhy, reason)
			}
		})
	}
}
