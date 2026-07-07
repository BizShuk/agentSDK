// Package memory owns the durability and bounded-context machinery of an
// agentsdk run.
//
// Memory has zero vendor dependencies for the core interface contracts; the
// only consumer of stdlib is filestore (JSON file I/O). Tests use the
// in-memory checkpointer + filestore backed by t.TempDir().
package memory

import "github.com/bizshuk/agentsdk/core"

// TokenCounter reports the token size of a slice of messages.
//
// Providers implement it natively (Anthropic / Google); openaicompat may
// fall back to a chars/4 heuristic. The default agentsdk does not bundle a
// tokenizer (no cgo tiktoken); callers wire one in.
type TokenCounter interface {
	Count(msgs []core.Message) int
}

// CharHeuristicCounter is the fallback token counter (chars/4) for
// providers without a native endpoint. It is intentionally dumb and
// deterministic — good enough for budget sanity checks, not for accuracy.
type CharHeuristicCounter struct{}

// Count returns len(text)/4 + 1 for every chunk of text-ish content.
func (CharHeuristicCounter) Count(msgs []core.Message) int {
	n := 0
	for _, m := range msgs {
		for _, c := range m.Parts {
			if c.Kind == core.PART_KIND_PLAIN_TEXT {
				// +1 so an empty message still registers 1 token.
				n += len(c.Text)/4 + 1
			}
		}
	}
	return n
}

// Window keeps the most recent N messages or, if TokenCounter is set,
// the most recent tokens <= MaxTokens.
//
// Window is intentionally not safe for concurrent mutation. The runtime
// loop is the single writer; the StateStore / Checkpointer serialize
// snapshots.
type Window struct {
	MaxMessages int
	MaxTokens   int
	Counter     TokenCounter
}

// Trim returns the suffix of msgs that fits the window. If both
// MaxMessages and MaxTokens are zero, no trimming occurs.
func (w Window) Trim(msgs []core.Message) []core.Message {
	if w.MaxMessages == 0 && w.MaxTokens == 0 {
		return msgs
	}
	if w.MaxMessages > 0 && len(msgs) > w.MaxMessages {
		msgs = msgs[len(msgs)-w.MaxMessages:]
	}
	if w.MaxTokens > 0 && w.Counter != nil {
		// Walk backwards dropping oldest until we fit.
		for len(msgs) > 1 && w.Counter.Count(msgs) > w.MaxTokens {
			msgs = msgs[1:]
		}
	}
	return msgs
}

// TrimInPlace mutates s.Messages in place.
func (w Window) TrimInPlace(s *core.State) {
	if s == nil {
		return
	}
	s.Messages = w.Trim(s.Messages)
}