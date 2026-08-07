// SSE parser for the xAI Grok chat-completions stream. The wire shape is
// the same as OpenAI's — each event is a `data: <json>` line where the
// payload matches StreamChunk in dto.go. End of stream is `data: [DONE]`.

package grok

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/protocol/sse"
)

// ParseStream reads SSE from r and feeds core.ModelChunk events into the
// returned channel. The terminal chunk carries Done=true; the runtime
// folds those into ModelResult.
//
// Unknown lines and malformed JSON are skipped, not failed. This matches
// the behavior of pi's openai-compat stream parser and keeps a single
// dropped delta from killing an otherwise healthy stream.
func ParseStream(ctx context.Context, r io.Reader) <-chan core.ModelChunk {
	out := make(chan core.ModelChunk, 16)

	go func() {
		defer close(out)
		decoder := sse.NewDecoder(r)
		var usage core.TokenUsage

		for {
			if ctx.Err() != nil {
				return
			}
			frame, err := decoder.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return
			}
			payload := strings.TrimSpace(string(frame.Data))
			if payload == "" || payload == "[DONE]" {
				if payload == "[DONE]" {
					break
				}
				continue
			}
			var chunk StreamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				continue
			}
			if chunk.Usage != nil {
				usage = core.TokenUsage{
					InputTokens:          chunk.Usage.InputTokens,
					OutputTokens:         chunk.Usage.OutputTokens,
					InputCacheReadTokens: chunk.Usage.InputDetails.CachedTokens,
					TotalTokens:          chunk.Usage.TotalTokens,
				}
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta
			if delta.ReasoningContent != "" {
				select {
				case out <- core.ModelChunk{Kind: core.PART_KIND_REASONING, Text: delta.ReasoningContent}:
				case <-ctx.Done():
					return
				}
			}
			if delta.Content != "" {
				select {
				case out <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Text: delta.Content}:
				case <-ctx.Done():
					return
				}
			}
		}

		// Terminal sentinel.
		select {
		case out <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Usage: usage, Done: true}:
		case <-ctx.Done():
		}
	}()

	return out
}
