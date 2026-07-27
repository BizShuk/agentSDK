package openaichat

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// ParseStream reads an OpenAI-compatible SSE stream and projects it into
// canonical model chunks. Malformed events are skipped. A clean EOF emits a
// terminal chunk; scanner failures and context cancellation close without one.
func ParseStream(ctx context.Context, reader io.Reader) (<-chan core.ModelChunk, error) {
	out := make(chan core.ModelChunk, 16)
	go func() {
		defer close(out)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
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

		if scanner.Err() != nil {
			return
		}
		emit(ctx, out, core.ModelChunk{Done: true})
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
