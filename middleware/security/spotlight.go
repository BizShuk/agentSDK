package security

import (
	"context"
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
// effects (TOOL_RESULT chunks) with the spotlight markers above.
//
// Implementation note: the dispatcher in runtime.Loop currently
// produces TOOL_RESULT inside its CALL_TOOL effect's natural handling;
// Spotlight's job is on the *output* side. We rewrite the model-visible
// transcript entry by emitting a NOTIFY effect that records the raw
// tool text, while the original ToolResult is left intact in state for
// the model to see with markers. For M3 this layer only inserts the
// marker text into ToolResult.Output (when output is a string/bytes);
// structured outputs are wrapped at the JSON layer.
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
			// Mutate the returned ToolResult.Output by wrapping its
			// text form with the spotlight markers.
			wrapped := wrapToolOutput(in.ToolResult.Output)
			if wrapped != nil {
				in.ToolResult.Output = wrapped
			}
			return s, in, term, nil
		}
	}
}

// wrapToolOutput returns the spotlight-wrapped form of v when v can
// be meaningfully wrapped (string / []byte / json-marshalable),
// otherwise nil. The original ToolResult carries Output as `any`.
func wrapToolOutput(v any) any {
	switch x := v.(type) {
	case string:
		return SpotlightOpen + x + SpotlightClose
	case []byte:
		return []byte(SpotlightOpen + string(x) + SpotlightClose)
	}
	// Fall back to JSON-marshaled output.
	data, err := marshalForSpotlight(v)
	if err != nil {
		return nil
	}
	return SpotlightOpen + string(data) + SpotlightClose
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