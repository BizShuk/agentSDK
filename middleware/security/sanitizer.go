package security

import (
	"context"
	"regexp"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// Sanitizer matches adversarial instructions in tool output and
// replaces the offending text with a banner, so the LLM sees a clear
// signal that this content should NOT be obeyed as instructions.
//
// Patterns are deliberately conservative — better to flag a benign
// string than to miss a real injection. False positives will appear in
// NOTIFY (level=warn) so the operator can audit.
type Sanitizer struct {
	// Patterns is the list of compiled regex to match against the
	// TOOL_RESULT text. When a match fires, the entire chunk text is
	// replaced with a sanitize banner.
	Patterns []*regexp.Regexp
	// WhyFor returns a reason string per pattern index for diagnosis.
	WhyFor []string
}

// DefaultSanitizer returns a Sanitizer with the standard injection
// pattern set: "ignore previous instructions", system override
// attempts, and explicit role-jailbreak phrases.
func DefaultSanitizer() *Sanitizer {
	patterns := []string{
		`(?i)ignore (all|any|previous|above) instructions`,
		`(?i)disregard (all|any|previous|above) (instructions|rules|prompts)`,
		`(?i)you (must|should|will) now`,
		`(?i)system\s*:\s*`,
		`(?i)new\s+instructions\s*:`,
		`(?i)forget (everything|all) (above|prior|previous)`,
		`(?i)\bexec\s+command\b`,
		`(?i)<\|.*?\|>`, // special tokens leaked by some providers
	}
	whys := []string{
		"ignore previous instructions",
		"disregard instructions",
		"command override",
		"system prefix",
		"new instructions",
		"forget prior context",
		"exec command",
		"special token leak",
	}
	s := &Sanitizer{WhyFor: make([]string, len(patterns))}
	for i, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue // skip invalid patterns
		}
		s.Patterns = append(s.Patterns, re)
		s.WhyFor[i] = whys[i]
	}
	return s
}

// Inspect returns a non-empty reason and the matched range if any
// pattern hits the input. Empty string / -1 mean clean.
func (s *Sanitizer) Inspect(text string) (reason string, matched bool) {
	if s == nil || text == "" {
		return "", false
	}
	for i, re := range s.Patterns {
		if loc := re.FindStringIndex(text); loc != nil {
			return s.matchReason(i, loc), true
		}
	}
	return "", false
}

func (s *Sanitizer) matchReason(i int, loc []int) string {
	if i < len(s.WhyFor) {
		why := s.WhyFor[i]
		if loc[1]-loc[0] < 80 {
			return why + " at <redacted>"
		}
		return why + " (offset " + itoa(loc[0]) + ")"
	}
	return "matched pattern " + itoa(i)
}

// Middleware returns a middleware that inspects CALL_TOOL return-path
// text and replaces matched content with the sanitizer banner. The
// replaced ToolResult.Output is also tagged via SanitizedTag so the
// spotlight wrappers show up coherently.
func (s *Sanitizer) Middleware() middleware.Middleware {
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Effect) (core.State, *core.Input, bool, error) {
			if eff.Kind != core.EFFECT_CALL_TOOL {
				return next(ctx, state, eff)
			}
			st, in, term, err := next(ctx, state, eff)
			if err != nil || in == nil || in.ToolResult == nil {
				return st, in, term, err
			}
			text := outputToString(in.ToolResult.Output)
			if text == "" {
				return st, in, term, nil
			}
			reason, hit := s.Inspect(text)
			if !hit {
				return st, in, term, nil
			}
			in.ToolResult.Output = FormatSanitized(reason) + " original_len=" + itoa(len(text))
			_ = text
			// Emit NOTIFY so operator sees the hit (separate effect via state).
			if st.Scratch == nil {
				st.Scratch = make(map[string]any, 4)
			}
			st.Scratch["sanitizer.last_reason"] = reason
			return st, in, term, nil
		}
	}
}

// outputToString best-effort string conversion of ToolResult.Output.
func outputToString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}
	return ""
}

// textSnippet returns a short excerpt around the match.
func textSnippet(s string, loc []int) string {
	a := maxInt(0, loc[0]-10)
	b := minInt(len(s), loc[1]+10)
	return s[a:b]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// strings contains is here to silence unused-import warnings if we
// later swap textSnippet for strings.Trim.
var _ = strings.Contains