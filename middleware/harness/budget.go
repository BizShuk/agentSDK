package harness

import (
	"context"
	"errors"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// BudgetExceededError is returned when the per-run Budget has been
// tripped. The runtime treats it as a fatal loop exit (state.Status =
// RUN_STATUS_FAILED).
type BudgetExceededError struct {
	Reason string // "turn_budget" | "round_budget" | "token_budget" | "wall_time_budget"
}

// Error implements error.
func (e *BudgetExceededError) Error() string {
	return "budget exceeded: " + e.Reason
}

// Is allows errors.Is(err, &BudgetExceededError{}) comparisons.
func (e *BudgetExceededError) Is(target error) bool {
	_, ok := target.(*BudgetExceededError)
	return ok
}

// ErrBudgetExceeded is the sentinel value.
var ErrBudgetExceeded = errors.New("budget exceeded")

// Budget is a Middleware that consults state.Budget before each dispatch.
// Unlike Retry / Timeout, this layer never invokes the inner — when the
// budget is exhausted, it surfaces a non-retryable error directly.
//
// State.Budget is advanced by the runtime before Step (Turn + UsedTurns)
// so the middleware sees the current usage without re-counting.
func Budget() middleware.Middleware {
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
			if exceeded, why := state.Budget.Exceeded(); exceeded {
				return state, nil, false, &BudgetExceededError{Reason: why}
			}
			return next(ctx, state, eff)
		}
	}
}

// IsBudgetExceeded is a small helper for callers that want to detect the
// loop exiting for budget reasons. errors.As(err, &BudgetExceededError{})
// is the canonical check.
func IsBudgetExceeded(err error) bool {
	var be *BudgetExceededError
	return errors.As(err, &be)
}