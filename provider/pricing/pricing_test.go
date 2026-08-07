package pricing_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/pricing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const manifestFixture = `{
  "data": [{
    "id": "meta/muse-spark-1.2",
    "canonical_slug": "meta/muse-spark-1.2-20260805",
    "pricing": {
      "prompt": "0.00000125",
      "completion": "0.00000425",
      "web_search": "0.0025",
      "input_cache_read": "0.00000015",
      "overrides": [{
        "min_prompt_tokens": 1000,
        "prompt": "0.00000250",
        "completion": "0.00000850"
      }]
    }
  }]
}`

func TestDecodeOpenRouterManifestKeepsBillingUnits(t *testing.T) {
	now := time.Date(2026, 8, 7, 1, 2, 3, 0, time.UTC)
	snapshot, err := pricing.DecodeOpenRouterManifest(strings.NewReader(manifestFixture), now)
	require.NoError(t, err)

	assert.Equal(t, pricing.OPENROUTER_MODELS_URL, snapshot.Source)
	assert.Equal(t, "2026-08-07T01:02:03Z", snapshot.PricingAsOf)
	rate, ok := snapshot.Models["meta/muse-spark-1.2"]
	require.True(t, ok)
	assert.Equal(t, "0.00000125", rate.Prompt)
	assert.Equal(t, "0.00000425", rate.Completion)
	assert.Equal(t, "0.0025", rate.WebSearch)
	assert.Equal(t, "0.00000015", rate.InputCacheRead)
	require.Len(t, rate.Overrides, 1)
	assert.Equal(t, 1000, rate.Overrides[0].MinPromptTokens)
}

func TestEstimateUsesNonCachedInputCacheOutputAndWebSearchUnits(t *testing.T) {
	snapshot, err := pricing.DecodeOpenRouterManifest(strings.NewReader(manifestFixture), time.Unix(0, 0).UTC())
	require.NoError(t, err)

	cost := snapshot.Estimate("meta", "muse-spark-1.2", core.TokenUsage{
		InputTokens:          100,
		InputCacheReadTokens: 40,
		OutputTokens:         20,
		WebSearchCount:       2,
		TotalTokens:          120,
	})

	assert.Equal(t, "0.0051660000", cost.AmountUSD)
	assert.Equal(t, core.COST_STATUS_ESTIMATED, cost.Status)
	assert.Equal(t, pricing.OPENROUTER_MODELS_URL, cost.Source)
}

func TestEstimateUsesLargestMatchingPromptTier(t *testing.T) {
	snapshot, err := pricing.DecodeOpenRouterManifest(strings.NewReader(manifestFixture), time.Unix(0, 0).UTC())
	require.NoError(t, err)

	cost := snapshot.Estimate("meta", "muse-spark-1.2", core.TokenUsage{
		InputTokens:  1000,
		OutputTokens: 10,
	})

	assert.Equal(t, "0.0025850000", cost.AmountUSD)
}

func TestEstimateReturnsUnpricedForUnknownOrInvalidUsage(t *testing.T) {
	snapshot, err := pricing.DecodeOpenRouterManifest(strings.NewReader(manifestFixture), time.Unix(0, 0).UTC())
	require.NoError(t, err)

	unknown := snapshot.Estimate("meta", "missing", core.TokenUsage{InputTokens: 1})
	assert.Equal(t, core.UnpricedCost(), unknown)

	invalid := snapshot.Estimate("meta", "muse-spark-1.2", core.TokenUsage{
		InputTokens: 1, InputCacheReadTokens: 2,
	})
	assert.Equal(t, core.UnpricedCost(), invalid)
}

func TestEstimateDoesNotTreatMissingUsageAsFree(t *testing.T) {
	snapshot, err := pricing.DecodeOpenRouterManifest(strings.NewReader(manifestFixture), time.Unix(0, 0).UTC())
	require.NoError(t, err)

	assert.Equal(t, core.UnpricedCost(), snapshot.Estimate("meta", "muse-spark-1.2", core.TokenUsage{}))
}

func TestEstimateAlwaysTreatsOllamaAsFree(t *testing.T) {
	snapshot, err := pricing.DecodeOpenRouterManifest(strings.NewReader(manifestFixture), time.Unix(0, 0).UTC())
	require.NoError(t, err)

	assert.Equal(t, core.FreeCost(), snapshot.Estimate("ollama", "muse-spark-1.2", core.TokenUsage{InputTokens: 1000}))
}

func TestDecodeOpenRouterManifestSkipsVariablePriceSentinels(t *testing.T) {
	snapshot, err := pricing.DecodeOpenRouterManifest(strings.NewReader(`{
      "data": [
        {"id":"openrouter/auto-beta","pricing":{"prompt":"-1","completion":"-1"}},
        {"id":"valid/model","pricing":{"prompt":"0.1","completion":"0.2"}}
      ]
    }`), time.Unix(0, 0).UTC())
	require.NoError(t, err)
	assert.NotContains(t, snapshot.Models, "openrouter/auto-beta")
	assert.Contains(t, snapshot.Models, "valid/model")
}
