package provider_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/anthropic"
	"github.com/bizshuk/agentsdk/provider/antigravity"
	"github.com/bizshuk/agentsdk/provider/codex"
	"github.com/bizshuk/agentsdk/provider/grok"
	"github.com/bizshuk/agentsdk/provider/minimax"
	"github.com/bizshuk/agentsdk/provider/protocol/sse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type streamParser func(context.Context, io.Reader) <-chan core.ModelChunk

type streamContract struct {
	name          string
	parse         streamParser
	deltaFrame    string
	terminalFrame string
	terminalChunk core.ModelChunk
}

func TestProviderStreamContracts(t *testing.T) {
	for _, contract := range providerStreamContracts() {
		t.Run(contract.name, func(t *testing.T) {
			t.Run("terminal", func(t *testing.T) {
				chunks := drainChunks(t, contract.parse(
					context.Background(),
					strings.NewReader(contract.terminalFrame),
				))

				assert.Equal(t, []core.ModelChunk{contract.terminalChunk}, chunks)
			})

			t.Run("transport_error", func(t *testing.T) {
				errTransport := errors.New("transport read failed")
				chunks := drainChunks(t, contract.parse(
					context.Background(),
					&errorAfterReader{
						data: []byte(contract.deltaFrame),
						err:  errTransport,
					},
				))

				require.Len(t, chunks, 1)
				assert.Equal(t, "before error", chunks[0].Text)
				assert.False(t, chunks[0].Done)
			})

			t.Run("cancellation", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				chunks := contract.parse(ctx, contextReader{ctx: ctx})
				cancel()

				assert.Empty(t, drainChunks(t, chunks))
			})
		})
	}
}

func TestAnthropicStreamDecodesMultilineData(t *testing.T) {
	raw := "event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"delta\":\n" +
		"data: {\"type\":\"text_delta\",\"text\":\"multiline\"}}\n\n" +
		anthropicTerminalFrame

	chunks := drainChunks(t, parseAnthropicStream(context.Background(), strings.NewReader(raw)))

	require.Len(t, chunks, 2)
	assert.Equal(t, "multiline", chunks[0].Text)
	assert.True(t, chunks[1].Done)
}

func TestAntigravityStreamRejectsPartialFrame(t *testing.T) {
	raw := antigravityDeltaFrame +
		"data: {\"response\":{\"candidates\":[]}}\n"

	chunks := drainChunks(t, parseAntigravityStream(context.Background(), strings.NewReader(raw)))

	require.Len(t, chunks, 1)
	assert.Equal(t, "before error", chunks[0].Text)
	assert.False(t, chunks[0].Done)
}

func TestCodexStreamRejectsFrameOverSizeLimit(t *testing.T) {
	raw := codexDeltaFrame +
		"data: " + strings.Repeat("x", int(sse.MAX_FRAME_BYTES)) + "\n\n"

	chunks := drainChunks(t, parseCodexStream(context.Background(), strings.NewReader(raw)))

	require.Len(t, chunks, 1)
	assert.Equal(t, "before error", chunks[0].Text)
	assert.False(t, chunks[0].Done)
}

func TestGrokStreamStripsUTF8BOM(t *testing.T) {
	raw := "\xef\xbb\xbf" + grokDeltaFrame + grokTerminalFrame

	chunks := drainChunks(t, grok.ParseStream(context.Background(), strings.NewReader(raw)))

	require.Len(t, chunks, 2)
	assert.Equal(t, "before error", chunks[0].Text)
	assert.True(t, chunks[1].Done)
}

const (
	anthropicDeltaFrame    = "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"before error\"}}\n\n"
	anthropicTerminalFrame = "event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	// Cloud Code streams whole GenerateResponse values, not deltas, and
	// closes without a terminal event — the last frame is simply the one
	// carrying finishReason.
	antigravityDeltaFrame    = "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"before error\"}]}}]}}\n\n"
	antigravityTerminalFrame = "data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[]},\"finishReason\":\"STOP\"}]}}\n\n"

	codexDeltaFrame    = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"before error\"}\n\n"
	codexTerminalFrame = "event: response.completed\n" +
		"data: {\"type\":\"response.completed\"}\n\n"

	grokDeltaFrame    = "data: {\"choices\":[{\"delta\":{\"content\":\"before error\"}}]}\n\n"
	grokTerminalFrame = "data: [DONE]\n\n"

	minimaxDeltaFrame    = "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"before error\"}}\n\n"
	minimaxTerminalFrame = "event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
)

func providerStreamContracts() []streamContract {
	return []streamContract{
		{
			name:          "anthropic",
			parse:         parseAnthropicStream,
			deltaFrame:    anthropicDeltaFrame,
			terminalFrame: anthropicTerminalFrame,
			terminalChunk: core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Done: true},
		},
		{
			name:          "antigravity",
			parse:         parseAntigravityStream,
			deltaFrame:    antigravityDeltaFrame,
			terminalFrame: antigravityTerminalFrame,
			terminalChunk: core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Done: true},
		},
		{
			name:          "codex",
			parse:         parseCodexStream,
			deltaFrame:    codexDeltaFrame,
			terminalFrame: codexTerminalFrame,
			terminalChunk: core.ModelChunk{Done: true},
		},
		{
			name:          "grok",
			parse:         grok.ParseStream,
			deltaFrame:    grokDeltaFrame,
			terminalFrame: grokTerminalFrame,
			terminalChunk: core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Done: true},
		},
		{
			name:          "minimax",
			parse:         minimax.ParseStream,
			deltaFrame:    minimaxDeltaFrame,
			terminalFrame: minimaxTerminalFrame,
			terminalChunk: core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Done: true},
		},
	}
}

func parseAnthropicStream(ctx context.Context, reader io.Reader) <-chan core.ModelChunk {
	chunks, _ := anthropic.ParseStream(ctx, reader)
	return chunks
}

func parseAntigravityStream(ctx context.Context, reader io.Reader) <-chan core.ModelChunk {
	chunks, _ := antigravity.ParseStream(ctx, reader)
	return chunks
}

func parseCodexStream(ctx context.Context, reader io.Reader) <-chan core.ModelChunk {
	chunks, _ := codex.ParseStream(ctx, reader)
	return chunks
}

func drainChunks(t *testing.T, chunks <-chan core.ModelChunk) []core.ModelChunk {
	t.Helper()
	var out []core.ModelChunk
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				return out
			}
			out = append(out, chunk)
		case <-timeout.C:
			t.Fatal("stream parser did not close its channel")
		}
	}
}

type errorAfterReader struct {
	data []byte
	err  error
}

func (r *errorAfterReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

type contextReader struct {
	ctx context.Context
}

func (r contextReader) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}
