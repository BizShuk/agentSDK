package perception

import "github.com/bizshuk/agentsdk/core"

// NormalizeFunc converts a raw payload (as emitted by a Source) into a
// structured Message suitable for inclusion in state.Messages.
//
// This is the bridge between domain-specific Percept shapes (log lines,
// webhook bodies, filesystem events) and the LLM-friendly transcript.
type NormalizeFunc func(p core.Percept) core.Message

// Normalizer is a helper that applies a NormalizeFunc to each percept
// and appends the resulting Message to a transcript.
type Normalizer struct {
	Fn      NormalizeFunc
	MaxSize int // optional cap on retained messages (0 = unbounded)
}

// Apply returns the normalized message for the given percept.
func (n *Normalizer) Apply(p core.Percept) core.Message {
	if n.Fn == nil {
		// default: text passthrough — assume payload is a string
		text, _ := p.Payload.(string)
		return core.Message{
			Role: core.ROLE_USER,
			Chunks: []core.Chunk{
				{Kind: core.CHUNK_KIND_TEXT, Text: text},
			},
			Ts: p.ObservedAt,
		}
	}
	return n.Fn(p)
}