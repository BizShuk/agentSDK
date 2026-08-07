package core_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenUsageAddAggregatesEveryBillingUnit(t *testing.T) {
	left := core.TokenUsage{
		InputTokens:          10,
		OutputTokens:         4,
		InputCacheReadTokens: 3,
		WebSearchCount:       1,
		TotalTokens:          14,
	}
	right := core.TokenUsage{
		InputTokens:          7,
		OutputTokens:         2,
		InputCacheReadTokens: 1,
		WebSearchCount:       2,
		TotalTokens:          9,
	}

	assert.Equal(t, core.TokenUsage{
		InputTokens:          17,
		OutputTokens:         6,
		InputCacheReadTokens: 4,
		WebSearchCount:       3,
		TotalTokens:          23,
	}, left.Add(right))
}

func TestExactCostFromUSDTicksUsesTenDecimalPlaces(t *testing.T) {
	assert.Equal(t, core.Cost{
		AmountUSD: "0.0000012345",
		Status:    core.COST_STATUS_EXACT,
	}, core.ExactCostFromUSDTicks(12345))
}

func TestCostAddPreservesKnownSubtotalAndMarksUnknownAggregate(t *testing.T) {
	known := core.Cost{AmountUSD: "0.1250000000", Status: core.COST_STATUS_ESTIMATED}
	unknown := core.Cost{AmountUSD: "0.0000000000", Status: core.COST_STATUS_UNPRICED}

	got, err := known.Add(unknown)
	require.NoError(t, err)
	assert.Equal(t, "0.1250000000", got.AmountUSD)
	assert.Equal(t, core.COST_STATUS_UNPRICED, got.Status)
}

func TestCostAddRejectsInvalidDecimal(t *testing.T) {
	_, err := (core.Cost{AmountUSD: "invalid", Status: core.COST_STATUS_EXACT}).Add(core.FreeCost())
	require.ErrorContains(t, err, "amount_usd")
}

func TestCostCentsRoundsHalfUpOnceAtPersistenceBoundary(t *testing.T) {
	cost := core.Cost{AmountUSD: "1.2349999999", Status: core.COST_STATUS_ESTIMATED}

	got, err := cost.Cents()
	require.NoError(t, err)
	assert.Equal(t, int64(123), got)

	cost.AmountUSD = "1.2350000000"
	got, err = cost.Cents()
	require.NoError(t, err)
	assert.Equal(t, int64(124), got)
}
