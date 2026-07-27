package openaichat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/protocol/sse"
)

// ParseStream reads an OpenAI-compatible SSE stream and projects it into
// canonical model chunks. Malformed JSON events are skipped. A clean EOF emits
// a terminal chunk; framing failures and context cancellation close without one.
func ParseStream(ctx context.Context, reader io.Reader) (<-chan core.ModelChunk, error) {
	out := make(chan core.ModelChunk, 16)
	go func() {
		defer close(out)
		decoder := sse.NewDecoder(reader)

		for {
			if ctx.Err() != nil {
				return
			}
			frame, err := decoder.Next()
			if errors.Is(err, io.EOF) {
				emit(ctx, out, core.ModelChunk{Done: true})
				return
			}
			if err != nil {
				return
			}
			payload := strings.TrimSpace(string(frame.Data))
			if payload == "" {
				continue
			}
			if payload == "[DONE]" {
				if !emit(ctx, out, core.ModelChunk{Done: true}) {
					return
				}
				continue
			}

			var chunk streamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				continue
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.ReasoningContent != "" &&
					!emit(ctx, out, core.ModelChunk{
						Kind: core.PART_KIND_REASONING,
						Text: choice.Delta.ReasoningContent,
					}) {
					return
				}
				if choice.Delta.Content != "" &&
					!emit(ctx, out, core.ModelChunk{
						Kind: core.PART_KIND_PLAIN_TEXT,
						Text: choice.Delta.Content,
					}) {
					return
				}
				for _, call := range choice.Delta.ToolCalls {
					if !emit(ctx, out, core.ModelChunk{
						Kind: core.PART_KIND_TOOL_USE,
						ToolUse: &core.ToolCall{
							ID:   call.ID,
							Name: call.Function.Name,
							Args: parseArgs(call.Function.Arguments),
						},
					}) {
						return
					}
				}
			}
		}
	}()
	return out, nil
}

func emit(ctx context.Context, out chan<- core.ModelChunk, chunk core.ModelChunk) bool {
	select {
	case out <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
}

func parseArgs(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil
	}
	return args
}
