package openaichat

import (
	"context"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStreamReadsCompleteFrames(t *testing.T) {
	raw := "\xef\xbb\xbfevent: chat.completion.chunk\n" +
		"data: {\"choices\":[{\"delta\":\n" +
		"data: {\"content\":\"hello\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":4,\"total_tokens\":15,\"prompt_tokens_details\":{\"cached_tokens\":3}}}\n\n" +
		"data: [DONE]\n\n"

	chunks, err := ParseStream(context.Background(), strings.NewReader(raw))
	require.NoError(t, err)

	var got []core.ModelChunk
	for chunk := range chunks {
		got = append(got, chunk)
	}
	assert.Equal(t, []core.ModelChunk{
		{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hello"},
		{
			Done: true,
			Usage: core.TokenUsage{
				InputTokens:          11,
				OutputTokens:         4,
				InputCacheReadTokens: 3,
				TotalTokens:          15,
			},
		},
	}, got)
}

func TestParseStreamRejectsPartialFrame(t *testing.T) {
	raw := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n"

	chunks, err := ParseStream(context.Background(), strings.NewReader(raw))
	require.NoError(t, err)

	var got []core.ModelChunk
	for chunk := range chunks {
		got = append(got, chunk)
	}
	assert.Empty(t, got)
}
