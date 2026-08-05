package antigravity

// SSE parser for /v1internal:streamGenerateContent?alt=sse.
//
// Every frame is a complete GenerateResponse in the same envelope the
// blocking endpoint uses — there is no delta vocabulary, no event names,
// and no terminal event. The stream simply ends:
//
//	data: {"response":{"candidates":[{"content":{"parts":[{"text":"Hi"}]}}]}}
//
//	data: {"response":{"candidates":[{"content":{"parts":[{"thought":true,
//	      "text":"…","thoughtSignature":"…"}]}}]}}
//
//	data: {"response":{"candidates":[{"content":{"parts":[]},
//	      "finishReason":"STOP"}],"usageMetadata":{…}}}
//
// provider/protocol/sse owns frame boundaries; the terminal semantics
// below stay local to this package. A frame that fails to decode is
// skipped, not fatal — a truncated or malformed frame mid-stream should
// not discard the text already delivered.

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/protocol/sse"
)

// StreamStop is the terminal metadata the chunk channel cannot carry. The
// caller reads it once the channel is drained; runtime folds it into
// ModelResult.Usage / StopReason.
type StreamStop struct {
	StopReason string
	Usage      core.TokenUsage
}

// ParseStream reads SSE from r and feeds core.ModelChunk values into the
// returned channel. The terminal chunk carries Done=true and is emitted
// only on a clean end of stream — a transport error ends the channel
// without it, so a caller can tell a finished turn from a broken one.
func ParseStream(ctx context.Context, r io.Reader) (<-chan core.ModelChunk, *StreamStop) {
	out := make(chan core.ModelChunk, 16)
	stop := &StreamStop{}

	go func() {
		defer close(out)
		decoder := sse.NewBoundedDecoder(r, MAX_STREAM_FRAME_BYTES)

		// A turn that called a tool ends "tool_use" whatever the
		// finishReason says. Gemini reports STOP for such a turn, and
		// that frame arrives AFTER the functionCall frame — without this
		// flag the later frame would overwrite the verdict and the turn
		// would read as finished.
		sawToolUse := false

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
			body, err := Unwrap([]byte(payload))
			if err != nil {
				continue // malformed frame — keep what we already sent
			}
			if body.UsageMetadata != nil {
				stop.Usage = toUsage(body.UsageMetadata)
			}
			if len(body.Candidates) == 0 {
				continue
			}
			candidate := body.Candidates[0]
			if candidate.FinishReason != "" && !sawToolUse {
				stop.StopReason = StopReason(candidate.FinishReason, nil)
			}
			for _, chunk := range toChunks(candidate.Content.Parts) {
				if chunk.Kind == core.PART_KIND_TOOL_USE {
					sawToolUse = true
					stop.StopReason = "tool_use"
				}
				select {
				case out <- chunk:
				case <-ctx.Done():
					return
				}
			}
		}

		select {
		case out <- core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Done: true}:
		case <-ctx.Done():
		}
	}()

	return out, stop
}

// toChunks projects one frame's parts onto the chunk vocabulary. A
// thought part yields up to two chunks — the text, then its signature —
// because core.ModelChunk carries either text or reasoning state, and the
// signature must survive for the next turn's continuation.
func toChunks(parts []Part) []core.ModelChunk {
	var out []core.ModelChunk
	for _, p := range parts {
		switch {
		case p.FunctionCall != nil:
			call := core.ToolCall{
				ID:   p.FunctionCall.ID,
				Name: p.FunctionCall.Name,
				Args: p.FunctionCall.Args,
			}
			if call.ID == "" {
				call.ID = p.FunctionCall.Name
			}
			out = append(out, core.ModelChunk{Kind: core.PART_KIND_TOOL_USE, ToolUse: &call})

		case p.Thought:
			if p.Text != "" {
				out = append(out, core.ModelChunk{Kind: core.PART_KIND_REASONING, Text: p.Text})
			}
			if p.ThoughtSignature != "" {
				out = append(out, core.ModelChunk{
					Kind:      core.PART_KIND_REASONING,
					Reasoning: &core.ReasoningState{Signature: p.ThoughtSignature},
				})
			}

		case p.InlineData != nil:
			// Image-generating models (gemini-*-flash-image) return their
			// output here, and they are Gemini 3+, so Generate reaches
			// them through the SSE path — dropping this case loses the
			// entire answer.
			if raw := decodeInline(p.InlineData.Data); len(raw) > 0 {
				out = append(out, core.ModelChunk{
					Kind:      core.PART_KIND_IMAGE,
					Image:     raw,
					ImageMIME: p.InlineData.MIMEType,
				})
			}

		case p.Text != "":
			out = append(out, core.ModelChunk{Kind: core.PART_KIND_PLAIN_TEXT, Text: p.Text})
		}
	}
	return out
}

// FoldStream drains a chunk channel back into a single ModelResult.
//
// It exists because the blocking endpoint does not return thought parts:
// the gateway only emits reasoning over SSE. Generate therefore calls the
// streaming endpoint for thinking models and folds the result here, so
// callers get the same ModelResult either way.
func FoldStream(chunks <-chan core.ModelChunk, stop *StreamStop) core.ModelResult {
	var parts []core.Part

	appendText := func(kind core.PartKind, text string) {
		if n := len(parts); n > 0 && parts[n-1].Kind == kind && parts[n-1].ToolUse == nil {
			parts[n-1].Text += text
			return
		}
		parts = append(parts, core.Part{Kind: kind, Text: text})
	}

	for chunk := range chunks {
		switch chunk.Kind {
		case core.PART_KIND_TOOL_USE:
			if chunk.ToolUse != nil {
				call := *chunk.ToolUse
				parts = append(parts, core.Part{Kind: core.PART_KIND_TOOL_USE, ToolUse: &call})
			}
		case core.PART_KIND_IMAGE:
			if len(chunk.Image) > 0 {
				parts = append(parts, core.Part{
					Kind:      core.PART_KIND_IMAGE,
					Image:     chunk.Image,
					ImageMIME: chunk.ImageMIME,
				})
			}
		case core.PART_KIND_REASONING:
			if chunk.Reasoning != nil {
				// A signature-only chunk closes the reasoning part it
				// belongs to rather than opening a new one.
				if n := len(parts); n > 0 && parts[n-1].Kind == core.PART_KIND_REASONING {
					parts[n-1].Reasoning = &core.ReasoningState{Signature: chunk.Reasoning.Signature}
					continue
				}
				parts = append(parts, core.Part{
					Kind:      core.PART_KIND_REASONING,
					Reasoning: &core.ReasoningState{Signature: chunk.Reasoning.Signature},
				})
				continue
			}
			if chunk.Text != "" {
				appendText(core.PART_KIND_REASONING, chunk.Text)
			}
		default:
			if chunk.Text != "" {
				appendText(core.PART_KIND_PLAIN_TEXT, chunk.Text)
			}
		}
	}

	out := core.ModelResult{Parts: parts}
	if stop != nil {
		out.StopReason = stop.StopReason
		out.Usage = stop.Usage
	}
	if out.StopReason == "" {
		out.StopReason = StopReason("", parts)
	}
	return out.NormalizeContent()
}

// decodeInline turns a base64 inlineData payload back into bytes. An
// undecodable payload yields nil rather than an error: the rest of the
// response is still worth delivering.
func decodeInline(data string) []byte {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil
	}
	return raw
}
