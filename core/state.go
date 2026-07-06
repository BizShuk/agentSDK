// Package core is the pure state machine of agentsdk.
//
// core has zero vendor dependencies — only the Go standard library.
// All I/O is performed at the runtime shell; this package only describes
// state transitions and effects.
//
// Per the user's project convention, package-level constants are
// SCREAMING_SNAKE_CASE; staticcheck ST1003 needs an exclusion for const.
package core

import "time"

// AutonomyLevel is the graduated trust level for tool execution and mutation.
// L0 — fully manual (every action requires human approval)
// L1 — low-risk automatic, higher-risk gated (enterprise floor)
// L2 — most automatic, high-risk gated (default cap)
// L3 — minimal gating
// L4 — fully autonomous
type AutonomyLevel int

const (
	AUTONOMY_L0 AutonomyLevel = iota
	AUTONOMY_L1
	AUTONOMY_L2
	AUTONOMY_L3
	AUTONOMY_L4
)

// String renders the level in the canonical SCREAMING form.
func (a AutonomyLevel) String() string {
	switch a {
	case AUTONOMY_L0:
		return "L0"
	case AUTONOMY_L1:
		return "L1"
	case AUTONOMY_L2:
		return "L2"
	case AUTONOMY_L3:
		return "L3"
	case AUTONOMY_L4:
		return "L4"
	default:
		return "L?"
	}
}

// RunStatus tracks the lifecycle of a single run.
type RunStatus string

const (
	RUN_STATUS_RUNNING         RunStatus = "running"
	RUN_STATUS_PAUSED_APPROVAL RunStatus = "paused_for_approval"
	RUN_STATUS_COMPLETED       RunStatus = "completed"
	RUN_STATUS_FAILED          RunStatus = "failed"
	RUN_STATUS_ABORTED         RunStatus = "aborted"
)

// Terminal reports whether a status can no longer transition.
func (s RunStatus) Terminal() bool {
	switch s {
	case RUN_STATUS_COMPLETED, RUN_STATUS_FAILED, RUN_STATUS_ABORTED:
		return true
	default:
		return false
	}
}

// Budget caps the resources consumed by a single run.
type Budget struct {
	MaxTurns    int           `json:"max_turns"`
	UsedTurns   int           `json:"used_turns"`
	MaxTokens   int           `json:"max_tokens"`
	UsedTokens  int           `json:"used_tokens"`
	MaxWallTime time.Duration `json:"max_wall_time"`
	StartedAt   time.Time     `json:"started_at"`
	NowFunc     func() time.Time `json:"-"` // injectable clock; defaults to time.Now
}

// Exceeded reports whether any limit has been breached.
func (b Budget) Exceeded() (bool, string) {
	if b.MaxTurns > 0 && b.UsedTurns >= b.MaxTurns {
		return true, "turn_budget"
	}
	if b.MaxTokens > 0 && b.UsedTokens >= b.MaxTokens {
		return true, "token_budget"
	}
	if b.MaxWallTime > 0 {
		now := b.now()
		if !b.StartedAt.IsZero() && now.Sub(b.StartedAt) >= b.MaxWallTime {
			return true, "wall_time_budget"
		}
	}
	return false, ""
}

func (b Budget) now() time.Time {
	if b.NowFunc != nil {
		return b.NowFunc()
	}
	return time.Now()
}

// State is the serialized, persistent snapshot of a run.
// Every field must be JSON-marshalable; Checkpointer round-trips through State.
type State struct {
	RunID            string             `json:"run_id"`
	Turn             int                `json:"turn"`
	Autonomy         AutonomyLevel      `json:"autonomy"`
	ThinkingKind     ThinkingKind       `json:"thinking_kind"`
	Messages         []Message          `json:"messages"`
	Scratch          map[string]any     `json:"scratch,omitempty"`
	PendingApprovals []PendingApproval  `json:"pending_approvals,omitempty"`
	Budget           Budget             `json:"budget"`
	Status           RunStatus          `json:"status"`
	UpdatedAt        time.Time          `json:"updated_at"`
	LastInputSeq     int                `json:"last_input_seq"`
}

// Clone returns a deep-enough copy suitable for callers that mutate the
// returned state. Messages and PendingApprovals are deep-copied so callers
// can freely mutate any nested chunk without affecting the original.
// Scratch is shallow-copied (treated as opaque blob by the runtime).
func (s State) Clone() State {
	out := s
	if s.Messages != nil {
		msgs := make([]Message, len(s.Messages))
		for i, m := range s.Messages {
			msgs[i] = m
			if m.Chunks != nil {
				ch := make([]Chunk, len(m.Chunks))
				copy(ch, m.Chunks)
				msgs[i].Chunks = ch
			}
		}
		out.Messages = msgs
	}
	if s.Scratch != nil {
		sc := make(map[string]any, len(s.Scratch))
		for k, v := range s.Scratch {
			sc[k] = v
		}
		out.Scratch = sc
	}
	if s.PendingApprovals != nil {
		pa := make([]PendingApproval, len(s.PendingApprovals))
		copy(pa, s.PendingApprovals)
		out.PendingApprovals = pa
	}
	return out
}
