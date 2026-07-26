package core

import (
	"maps"
	"time"
)

// State is the serialized, persistent snapshot of a run.
// Every field must be JSON-marshalable; Recoverer round-trips through State.
//
// Field-name JSON tags are kept stable (scratch / thinking_kind) so
// persisted state from before the rename loads cleanly through the
// v1→v2 migration shim in memory/filestore.
type State struct {
	RunID            string            `json:"run_id"`
	Turn             int               `json:"turn"`
	Autonomy         AutonomyLevel     `json:"autonomy"`
	ReasoningStyle   ReasoningStyle    `json:"thinking_kind"` // tag preserved for back-compat
	Messages         []Message         `json:"messages"`
	WorkingMemory    map[string]any    `json:"scratch,omitempty"` // Go field renamed; wire tag kept
	PendingApprovals []PendingApproval `json:"pending_approvals,omitempty"`
	Budget           Budget            `json:"budget"`
	Status           RunStatus         `json:"status"`
	UpdatedAt        time.Time         `json:"updated_at"`
	LastInputSeq     int               `json:"last_input_seq"`
}

// Clone returns a deep-enough copy suitable for callers that mutate the
// returned state. Messages and PendingApprovals are deep-copied so callers
// can freely mutate any nested part without affecting the original.
// WorkingMemory is shallow-copied (treated as opaque blob by the runtime).
func (s State) Clone() State {
	out := s
	if s.Messages != nil {
		msgs := make([]Message, len(s.Messages))
		for i, m := range s.Messages {
			msgs[i] = m
			if m.Parts != nil {
				ps := make([]Part, len(m.Parts))
				copy(ps, m.Parts)
				msgs[i].Parts = ps
			}
		}
		out.Messages = msgs
	}
	if s.WorkingMemory != nil {
		wm := make(map[string]any, len(s.WorkingMemory))
		maps.Copy(wm, s.WorkingMemory)
		out.WorkingMemory = wm
	}
	if s.PendingApprovals != nil {
		pa := make([]PendingApproval, len(s.PendingApprovals))
		copy(pa, s.PendingApprovals)
		out.PendingApprovals = pa
	}
	return out
}
