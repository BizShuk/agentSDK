package minimax_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/minimax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStreamCarriesTerminalUsage(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`,
		``,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
		``,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n") + "\n"

	chunks := minimax.ParseStream(context.Background(), strings.NewReader(raw))
	var got []core.ModelChunk
	for chunk := range chunks {
		got = append(got, chunk)
	}

	require.Len(t, got, 1)
	assert.True(t, got[0].Done)
	assert.Equal(t, core.TokenUsage{
		InputTokens:  8,
		OutputTokens: 3,
		TotalTokens:  11,
	}, got[0].Usage)
}
