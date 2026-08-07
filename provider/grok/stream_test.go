package grok_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/grok"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStreamCarriesTerminalUsage(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"ok"}}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n") + "\n"

	chunks := grok.ParseStream(context.Background(), strings.NewReader(raw))
	var got []core.ModelChunk
	for chunk := range chunks {
		got = append(got, chunk)
	}

	require.Len(t, got, 2)
	assert.Equal(t, "ok", got[0].Text)
	assert.Equal(t, core.TokenUsage{
		InputTokens:  9,
		OutputTokens: 2,
		TotalTokens:  11,
	}, got[1].Usage)
	assert.True(t, got[1].Done)
}
