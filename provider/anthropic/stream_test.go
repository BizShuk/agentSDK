package anthropic_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStreamPreservesThinkingAndSignature(t *testing.T) {
	stream := strings.NewReader(strings.Join([]string{
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"inspect first"}}`,
		``,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}`,
		``,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"done"}}`,
		``,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n") + "\n")

	chunks, _ := anthropic.ParseStream(context.Background(), stream)
	var got []core.ModelChunk
	for chunk := range chunks {
		got = append(got, chunk)
	}

	require.Len(t, got, 4)
	assert.Equal(t, core.PART_KIND_REASONING, got[0].Kind)
	assert.Equal(t, "inspect first", got[0].Text)
	assert.Equal(t, core.PART_KIND_REASONING, got[1].Kind)
	require.NotNil(t, got[1].Reasoning)
	assert.Equal(t, "sig-1", got[1].Reasoning.Signature)
	assert.Equal(t, core.PART_KIND_PLAIN_TEXT, got[2].Kind)
	assert.Equal(t, "done", got[2].Text)
	assert.True(t, got[3].Done)
}
