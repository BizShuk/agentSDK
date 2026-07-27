package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

var errNoRunner = errors.New("agent: runner is required")

// Run executes bootstrap, engine rounds, and completion without taking
// ownership of the process. Bootstrap owns the complete engine and opening
// state; Run does not fill missing dependencies from Host.
func Run(ctx context.Context, a Runner, host *Host, opts ...RunOption) error {
	if a == nil {
		return errNoRunner
	}
	if host == nil {
		return errNoHost
	}
	if a.Name() == "" {
		return errors.New("agent: Runner.Name must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("agent: run context: %w", err)
	}

	o := DefaultRunOpts()
	for _, opt := range opts {
		opt(&o)
	}

	log := host.Logger
	if log == nil {
		log = slog.Default().With(slog.String("app", a.Name()))
	}

	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}

	engine, state, err := a.Bootstrap(ctx, host)
	if err != nil {
		return fmt.Errorf("agent: bootstrap: %w", err)
	}
	if engine == nil {
		return fmt.Errorf("agent: bootstrap: %w", errNilEngine)
	}

	start := time.Now()
	log.Info("run_start", "run_id", state.RunID)
	final, runErr := safeRun(ctx, engine, state)
	if runErr != nil {
		return fmt.Errorf("agent: engine run: %w", runErr)
	}

	if in, ok := a.(Interactive); ok {
		for {
			reason, asks := pauseReason(final)
			if !asks {
				break
			}
			res, err := resolveRound(ctx, in, o.RoundTimeout, Pause{State: final, Reason: reason})
			if err != nil {
				return fmt.Errorf("agent: next round (%s): %w", reason, err)
			}
			if res.Stop || (reason == PAUSE_ROUND_END && res.Input == "") {
				break
			}
			next, err := advance(ctx, engine, final, reason, res)
			if err != nil {
				return fmt.Errorf("agent: advance (%s): %w", reason, err)
			}
			final = next
			log.Info("round_advanced",
				"run_id", final.RunID,
				"reason", string(reason),
				"decided_by", res.By,
				"status", string(final.Status))
		}
	}

	if c, ok := a.(Completer); ok {
		if err := c.OnComplete(ctx, final); err != nil {
			return fmt.Errorf("agent: complete: %w", err)
		}
	}

	log.Info("run_done",
		"run_id", final.RunID,
		"dur_ms", time.Since(start).Milliseconds(),
		"turns", final.Turn,
		"rounds", final.Budget.UsedRounds,
		"status", string(final.Status))
	return nil
}

func pauseReason(s core.State) (PauseReason, bool) {
	switch s.Status {
	case core.RUN_STATUS_PAUSED_APPROVAL:
		return PAUSE_APPROVAL, true
	case core.RUN_STATUS_COMPLETED:
		return PAUSE_ROUND_END, true
	default:
		return "", false
	}
}

func resolveRound(ctx context.Context, in Interactive, d time.Duration, p Pause) (Resume, error) {
	if d <= 0 {
		return in.NextRound(ctx, p)
	}
	rctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return in.NextRound(rctx, p)
}

// advance submits approval directly because SubmitHumanDecision drives the
// engine. A completed round is reopened before its steered follow-up.
func advance(ctx context.Context, e *Engine, s core.State, reason PauseReason, res Resume) (core.State, error) {
	if res.Input != "" {
		e.Steer(res.Input)
	}
	if reason == PAUSE_APPROVAL {
		by := res.By
		if by == "" {
			by = "interactive"
		}
		decision := res.Decision
		if decision == "" {
			decision = core.APPROVAL_DECISION_REJECT
		}
		return e.SubmitHumanDecision(ctx, s.RunID, decision, by)
	}
	next := s.Clone()
	next.Status = core.RUN_STATUS_RUNNING
	delete(next.WorkingMemory, "think_then_act.phase")
	return e.Run(ctx, next)
}

// safeRun converts a loop panic into a persisted failed run.
func safeRun(ctx context.Context, e *Engine, state core.State) (final core.State, err error) {
	defer func() {
		rec := recover()
		if rec == nil {
			return
		}
		panicErr := fmt.Errorf("panic: %v\n%s", rec, debug.Stack())
		var persistErr error
		final, persistErr = markFailed(ctx, e, state)
		if persistErr != nil {
			err = errors.Join(panicErr, fmt.Errorf("persist failed state: %w", persistErr))
			return
		}
		err = panicErr
	}()
	return e.Run(ctx, state)
}

// markFailed reloads the latest checkpoint and persists a failed status on
// a cancellation-detached context.
func markFailed(ctx context.Context, e *Engine, seed core.State) (core.State, error) {
	out := seed
	out.Status = core.RUN_STATUS_FAILED
	out.UpdatedAt = time.Now().UTC()
	if e == nil || e.Store == nil {
		return out, nil
	}
	saveCtx := context.WithoutCancel(ctx)
	if latest, err := e.Store.Load(saveCtx, seed.RunID); err == nil {
		out = latest
		out.Status = core.RUN_STATUS_FAILED
		out.UpdatedAt = time.Now().UTC()
	}
	if err := e.Store.Save(saveCtx, out); err != nil {
		return out, err
	}
	return out, nil
}
