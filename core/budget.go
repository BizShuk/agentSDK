package core

import "time"

// Budget caps the resources consumed by a single run.
type Budget struct {
	MaxTurns  int `json:"max_turns"`
	UsedTurns int `json:"used_turns"`

	// MaxRounds caps CALL_MODEL dispatches. A round is one model request
	// plus every tool call it triggers. MaxTurns counts Decide iterations
	// and remains the internal runaway guard.
	MaxRounds  int `json:"max_rounds,omitempty"`
	UsedRounds int `json:"used_rounds,omitempty"`

	// MaxToolCalls caps operations within one round. Zero means unbounded.
	// Excess calls are settled with failed tool_result messages rather than
	// dropped, preserving the one-result-per-tool-use transcript invariant.
	MaxToolCalls int `json:"max_tool_calls,omitempty"`

	MaxTokens   int              `json:"max_tokens"`
	UsedTokens  int              `json:"used_tokens"`
	MaxWallTime time.Duration    `json:"max_wall_time"`
	StartedAt   time.Time        `json:"started_at"`
	NowFunc     func() time.Time `json:"-"` // injectable clock; defaults to time.Now
}

// Exceeded reports whether any limit has been breached.
func (b Budget) Exceeded() (bool, string) {
	if b.MaxTurns > 0 && b.UsedTurns >= b.MaxTurns {
		return true, "turn_budget"
	}
	if b.MaxRounds > 0 && b.UsedRounds >= b.MaxRounds {
		return true, "round_budget"
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
