package minimax

// SSE parser for the Anthropic-Messages-compatible stream exposed by
// minimax at https://api.minimax.io/anthropic/v1/messages. The wire
// shape is identical to Anthropic's documented SSE format:
//
//	event: message_start
//	data: {"type":"message_start","message":{...}}
//
//	event: content_block_start
//	data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}
//
//	event: content_block_delta
//	data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}
//
//	event: content_block_stop
//	data: {"type":"content_block_stop","index":0}
//
//	event: message_delta
//	data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":N}}
//
//	event: message_stop
//	data: {"type":"message_stop"}
//
//	event: ping
//	data: {"type":"ping"}
//
//	event: error
//	data: {"type":"error","error":{"type":"...","message":"..."}}
//
// provider/protocol/sse owns frame boundaries. We read the complete `data`
// payload and keep MiniMax terminal and JSON semantics local to this package.
// Unknown events are skipped, not failed.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/protocol/sse"
)

// StreamEvent is one SSE event from the minimax stream. The fields are
// intentionally optional — different events populate different subsets.
type StreamEvent struct {
	Type         string        `json:"type"`
	Index        int           `json:"index,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
	Delta        *StreamDelta  `json:"delta,omitempty"`
	Message      *Response     `json:"message,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
	Error        *StreamError  `json:"error,omitempty"`
}

// StreamDelta is the per-event delta payload. Its `type` distinguishes
// text_delta from input_json_delta (the latter carries partial tool args).
type StreamDelta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	Thinking   string `json:"thinking,omitempty"`
	Signature  string `json:"signature,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

// StreamError is the error event payload.
type StreamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ParseStream reads SSE from r and feeds core.ModelChunk events into the
// returned channel. The terminal chunk carries Done=true; the caller
// closes over the channel and folds the chunks into a ModelResult.
//
// The returned channel is closed by this function on completion or when
// ctx is cancelled.
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
				continue
			}
			var ev StreamEvent
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue // ignore malformed lines
			}
			switch ev.Type {
			case "message_start":
				if ev.Message != nil {
					usage.InputTokens = ev.Message.Usage.InputTokens
				}
			case "message_delta":
				if ev.Usage != nil {
					if ev.Usage.InputTokens != 0 {
						usage.InputTokens = ev.Usage.InputTokens
					}
					if ev.Usage.OutputTokens != 0 {
						usage.OutputTokens = ev.Usage.OutputTokens
					}
				}
			case "content_block_delta":
				chunk, ok := reasoningAwareChunk(ev.Delta)
				if !ok {
					continue
				}
				select {
				case out <- chunk:
				case <-ctx.Done():
					return
				}
			case "content_block_stop":
				if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
					select {
					case out <- core.ModelChunk{
						Kind: core.PART_KIND_TOOL_USE,
						ToolUse: &core.ToolCall{
							ID:   ev.ContentBlock.ID,
							Name: ev.ContentBlock.Name,
							Args: parseArgs(ev.ContentBlock.Input),
						},
					}:
					case <-ctx.Done():
						return
					}
				}
			case "error":
				if ev.Error != nil {
					select {
					case out <- core.ModelChunk{
						Kind: core.PART_KIND_PLAIN_TEXT,
						Text: fmt.Sprintf("[minimax error: %s]", ev.Error.Message),
						Done: true,
					}:
					case <-ctx.Done():
						return
					}
					return
				}
			}
		}

		// Terminal sentinel.
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		select {
		case out <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Usage: usage, Done: true}:
		case <-ctx.Done():
		}
	}()

	return out
}

func reasoningAwareChunk(delta *StreamDelta) (core.ModelChunk, bool) {
	if delta == nil {
		return core.ModelChunk{}, false
	}
	switch delta.Type {
	case "text_delta":
		return core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Text: delta.Text}, delta.Text != ""
	case "thinking_delta":
		return core.ModelChunk{Kind: core.PART_KIND_REASONING, Text: delta.Thinking}, delta.Thinking != ""
	case "signature_delta":
		return core.ModelChunk{
			Kind:      core.PART_KIND_REASONING,
			Reasoning: &core.ReasoningState{Signature: delta.Signature},
		}, delta.Signature != ""
	default:
		return core.ModelChunk{}, false
	}
}

func parseArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
