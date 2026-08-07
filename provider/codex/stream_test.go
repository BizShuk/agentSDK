package codex_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/codex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStreamCarriesCompletedUsage(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		``,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":6,"total_tokens":16,"input_tokens_details":{"cached_tokens":4}}}}`,
		``,
	}, "\n") + "\n"

	chunks, err := codex.ParseStream(context.Background(), strings.NewReader(raw))
	require.NoError(t, err)
	var got []core.ModelChunk
	for chunk := range chunks {
		got = append(got, chunk)
	}

	require.Len(t, got, 2)
	assert.Equal(t, "ok", got[0].Text)
	assert.Equal(t, core.TokenUsage{
		InputTokens:          10,
		OutputTokens:         6,
		InputCacheReadTokens: 4,
		TotalTokens:          16,
	}, got[1].Usage)
	assert.True(t, got[1].Done)
}
