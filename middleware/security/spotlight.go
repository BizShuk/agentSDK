package security

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// Spotlight markers wrap untrusted content. They are designed so the
// model can recognize tool output vs system messages at a glance, while
// humans reading logs can locate untrusted sections easily.
const (
	SpotlightOpen  = "<UNTRUSTED_TOOL_OUTPUT>\n"
	SpotlightClose = "\n</UNTRUSTED_TOOL_OUTPUT>"
	SanitizedTag   = "[SANITIZED_BY_AGENTSDK]"
)

// Spotlight returns a Middleware that wraps CALL_TOOL return-path
// results (the TOOL_RESULT the model sees next turn) with untrusted
// markers.
//
// Two shapes are handled:
//
//   - string / []byte that is NOT valid JSON → wrapped inline with the
//     SpotlightOpen / SpotlightClose text markers.
//   - everything else (incl. json.RawMessage and any marshalable value)
//     → wrapped at the JSON layer as
//     {"untrusted": true, "content": <original>}, which stays a single
//     valid JSON value. This is the path registered tools hit, since
//     tool.CallWithRawMessage returns Output as json.RawMessage (i.e. []byte).
//
// Plain []byte that happens to be valid JSON is treated as structured
// output (JSON layer); a non-JSON byte slice is treated as opaque text
// and wrapped inline.
func Spotlight() middleware.Middleware {
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
			if eff.Kind != core.INSTRUCTION_CALL_TOOL {
				return next(ctx, state, eff)
			}
			s, in, term, err := next(ctx, state, eff)
			if err != nil || in == nil || in.ToolResult == nil {
				return s, in, term, err
			}
			// Mutate the returned ToolResult.Output by wrapping it with
			// the spotlight markers / JSON envelope.
			if wrapped := wrapToolOutput(in.ToolResult.Output); wrapped != nil {
				in.ToolResult.Output = wrapped
				// Propagate the wrapped form into the transcript too: the
				// base dispatcher appends the *raw* tool result into
				// state.Messages before this middleware runs, so without
				// this sync the model would see the unwrapped output next
				// turn. Match on CallID to stay safe against concurrent or
				// re-entrant dispatch.
				syncTranscriptOutput(&s, in.ToolResult.CallID, wrapped)
			}
			return s, in, term, nil
		}
	}
}

// syncTranscriptOutput overwrites the Output of the last tool-result
// part in state.Messages whose CallID matches, with the wrapped value.
// It mutates state in place. No-op when there is no matching part.
//
// We only rewrite the final matching tool message — the base dispatcher
// appends exactly one tool message per CALL_TOOL, and middlewares wrap
// outer-to-inner so the relevant message is the most recent one.
func syncTranscriptOutput(state *core.State, callID string, wrapped any) {
	if state == nil || callID == "" {
		return
	}
	for i := len(state.Messages) - 1; i >= 0; i-- {
		parts := state.Messages[i].Parts
		for j := len(parts) - 1; j >= 0; j-- {
			tr := parts[j].ToolResult
			if tr != nil && tr.CallID == callID {
				tr.Output = wrapped
				return
			}
		}
	}
}

// wrapToolOutput returns the spotlight-wrapped form of v, or nil when v
// is nil (nothing to wrap). Output is `any` per core.ToolResult.
//
// String values are wrapped inline with the text markers — convenient for
// human-readable logs and for the model scanning for the untrusted banner.
//
// Byte slices that parse as JSON are wrapped at the JSON layer (so the
// output stays a valid JSON value); byte slices that are not JSON are
// wrapped inline like strings. Any other value is JSON-marshaled first
// then wrapped at the JSON layer.
func wrapToolOutput(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		return SpotlightOpen + x + SpotlightClose
	case []byte:
		return wrapBytes(x)
	}
	// Fall back to JSON-marshaled output, wrapped at the JSON layer.
	data, err := marshalForSpotlight(v)
	if err != nil {
		return nil
	}
	return wrapBytes(data)
}

// wrapBytes wraps a byte slice. If it is valid JSON, the wrapping is done
// at the JSON layer (producing a valid JSON object); otherwise it is
// wrapped inline with the text markers.
func wrapBytes(b []byte) any {
	if json.Valid(b) {
		return wrapJSONContent(b)
	}
	return []byte(SpotlightOpen + string(b) + SpotlightClose)
}

// wrapJSONContent returns a JSON-encoded object
// {"untrusted": true, "content": <rawJSON>} that carries the original
// structured output verbatim. rawJSON must already be valid JSON.
func wrapJSONContent(rawJSON []byte) json.RawMessage {
	// Build the envelope by hand to avoid re-parsing rawJSON: we only
	// need to splice it in as the value of "content". The two literal
	// segments are static JSON, so concatenation stays well-formed as
	// long as rawJSON is a single valid JSON value.
	const prefix = `{"untrusted":true,"content":`
	const suffix = `}`
	buf := make([]byte, 0, len(prefix)+len(rawJSON)+len(suffix))
	buf = append(buf, prefix...)
	buf = append(buf, rawJSON...)
	buf = append(buf, suffix...)
	return buf
}

// marshalForSpotlight is a tiny shim so the spotlight middleware does
// not import encoding/json at the top of the file (helps test
// visibility). It centralizes the JSON encoding.
func marshalForSpotlight(v any) ([]byte, error) {
	// Defer to json via a thin helper file; split out below.
	return marshalAny(v)
}

// FormatSanitized returns a human-friendly banner for a tool result
// that the sanitizer dropped / replaced.
func FormatSanitized(reason string) string {
	return fmt.Sprintf("%s reason=%q", SanitizedTag, reason)
}
