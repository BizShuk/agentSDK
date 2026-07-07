package perception

// TODO(M3): same as source.go — root-level scaffolding, no consumer yet.
// ToMessage.Apply is referenced only by its own test. Resolve at M3 boundary.

import "github.com/bizshuk/agentsdk/core"

// ToMessageFunc converts a raw payload (as emitted by an ObservationSource)
// into a structured Message suitable for inclusion in state.Messages.
//
// This is the bridge between domain-specific Observation shapes (log lines,
// webhook bodies, filesystem events) and the LLM-friendly transcript.
type ToMessageFunc func(p core.Observation) core.Message

// ToMessage is a helper that applies a ToMessageFunc to each observation
// and appends the resulting Message to a transcript.
type ToMessage struct {
	Fn      ToMessageFunc
	MaxSize int // optional cap on retained messages (0 = unbounded)
}

// Apply returns the normalized message for the given observation.
func (n *ToMessage) Apply(p core.Observation) core.Message {
	if n.Fn == nil {
		// default: text passthrough — assume payload is a string
		text, _ := p.Payload.(string)
		return core.Message{
			Role: core.ROLE_USER,
			Parts: []core.Part{
				{Kind: core.PART_KIND_PLAIN_TEXT, Text: text},
			},
			Ts: p.ObservedAt,
		}
	}
	return n.Fn(p)
}
