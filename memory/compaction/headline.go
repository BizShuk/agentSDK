package compaction

import (
	"strings"

	"github.com/bizshuk/agentsdk/core"
)

// HeadlineCompactor concatenates the first line of every text chunk into
// a single assistant message. Deterministic, no I/O — used by tests and
// as a fallback when the provider is unavailable.
type HeadlineCompactor struct{}

// Compact implements Compactor.
func (HeadlineCompactor) Compact(msgs []core.Message) (core.Message, error) {
	var b strings.Builder
	b.WriteString("[compacted summary] ")
	wrote := 0
	for _, m := range msgs {
		for _, c := range m.Parts {
			if c.Kind != core.PART_KIND_PLAIN_TEXT {
				continue
			}
			line := c.Text
			if idx := strings.IndexByte(line, '\n'); idx >= 0 {
				line = line[:idx]
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if wrote > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(line)
			wrote++
		}
	}
	if wrote == 0 {
		b.WriteString("(empty)")
	}
	return core.Message{
		Role: core.ROLE_ASSISTANT,
		Parts: []core.Part{
			{Kind: core.PART_KIND_PLAIN_TEXT, Text: b.String()},
		},
	}, nil
}
