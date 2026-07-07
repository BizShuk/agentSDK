package harness

import (
	"context"
	"errors"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// TimeoutConfig sets a per-effect deadline. Effects that exceed the
// deadline are cancelled and the inner Next returns ctx.Err().
type TimeoutConfig struct {
	PerEffect time.Duration
	// OnTimeout, if set, is called with the effect that exceeded the
	// deadline. Useful for observability / metrics.
	OnTimeout func(eff core.Instruction)
}

// Timeout returns a Middleware that bounds the time the inner dispatcher
// can spend on a single effect. The inner context is derived from ctx
// with a per-call WithTimeout.
//
// Errors returned by the inner that stem from ctx.DeadlineExceeded are
// surfaced as-is — they are NOT marked retryable by the Retry middleware
// unless the provider explicitly classifies them as transient.
func Timeout(cfg TimeoutConfig) middleware.Middleware {
	per := cfg.PerEffect
	if per <= 0 {
		per = 60 * time.Second
	}
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
			cctx, cancel := context.WithTimeout(ctx, per)
			defer cancel()
			s, in, term, err := next(cctx, state, eff)
			if errors.Is(cctx.Err(), context.DeadlineExceeded) {
				if cfg.OnTimeout != nil {
					cfg.OnTimeout(eff)
				}
				if err == nil {
					return s, in, term, cctx.Err()
				}
			}
			return s, in, term, err
		}
	}
}