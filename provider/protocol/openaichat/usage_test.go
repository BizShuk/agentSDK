package openaichat_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/protocol/openaichat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeResponsePreservesCachedInputTokens(t *testing.T) {
	result, err := openaichat.DecodeResponse([]byte(`{
      "choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],
      "usage":{
        "prompt_tokens":10,
        "completion_tokens":2,
        "total_tokens":12,
        "prompt_tokens_details":{"cached_tokens":4}
      }
    }`))
	require.NoError(t, err)
	assert.Equal(t, core.TokenUsage{
		InputTokens:          10,
		OutputTokens:         2,
		InputCacheReadTokens: 4,
		TotalTokens:          12,
	}, result.Usage)
}
