package agent

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// Deprecated: exit-code constants moved to agent/cli. The values are
// unchanged (0 / 1); new code should import cli.EXIT_OK / cli.EXIT_ERROR.
const (
	EXIT_OK    = 0
	EXIT_ERROR = 1
)

// Run executes preflight, bootstrap, engine rounds, and completion without
// taking ownership of the process.
func Run(ctx context.Context, a Runner, host *Host, opts ...RunOption) int {
	o := DefaultRunOpts()
	for _, opt := range opts {
		opt(&o)
	}

	log := host.Logger
	if log == nil {
		log = slog.Default().With(slog.String("app", a.Name()))
	}

	if a.Name() == "" {
		log.Error("run: Runner.Name must not be empty")
		return EXIT_ERROR
	}

	if err := ctx.Err(); err != nil {
		log.Error("run context cancelled before bootstrap", "err", err)
		return EXIT_ERROR
	}

	if p, ok := a.(Preflighter); ok {
		if err := p.Preflight(ctx, host); err != nil {
			log.Error("preflight failed", "err", err)
			return EXIT_ERROR
		}
	}

	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}

	engine, state, err := a.Bootstrap(ctx, host)
	if err != nil {
		log.Error("bootstrap failed", "err", err)
		return EXIT_ERROR
	}
	if engine == nil {
		log.Error("bootstrap returned a nil Engine")
		return EXIT_ERROR
	}
	bindHost(host, engine, &state)

	start := time.Now()
	log.Info("run_start", "run_id", state.RunID)
	final, runErr := safeRun(ctx, engine, state)
	if runErr != nil {
		log.Error("run_failed",
			"run_id", state.RunID,
			"dur_ms", time.Since(start).Milliseconds(),
			"turns", final.Turn,
			"err", runErr)
		return EXIT_ERROR
	}

	if in, ok := a.(Interactive); ok {
		for {
			reason, asks := pauseReason(final)
			if !asks {
				break
			}
			res, err := resolveRound(ctx, in, o.RoundTimeout, Pause{State: final, Reason: reason})
			if err != nil {
				log.Error("next_round failed",
					"run_id", final.RunID, "reason", string(reason), "err", err)
				return EXIT_ERROR
			}
			if res.Stop || (reason == PAUSE_ROUND_END && res.Input == "") {
				break
			}
			next, err := advance(ctx, engine, final, reason, res)
			if err != nil {
				log.Error("advance failed",
					"run_id", final.RunID, "reason", string(reason), "err", err)
				return EXIT_ERROR
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
			log.Error("on_complete failed", "run_id", final.RunID, "err", err)
			return EXIT_ERROR
		}
	}

	log.Info("run_done",
		"run_id", final.RunID,
		"dur_ms", time.Since(start).Milliseconds(),
		"turns", final.Turn,
		"rounds", final.Budget.UsedRounds,
		"status", string(final.Status))
	return EXIT_OK
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
		err = fmt.Errorf("panic: %v", rec)
		slog.Error("run_panic", "run_id", state.RunID, "stack", string(debug.Stack()))
		final = markFailed(ctx, e, state)
	}()
	return e.Run(ctx, state)
}

// markFailed reloads the latest checkpoint and persists a failed status on
// a cancellation-detached context.
func markFailed(ctx context.Context, e *Engine, seed core.State) core.State {
	out := seed
	out.Status = core.RUN_STATUS_FAILED
	out.UpdatedAt = time.Now().UTC()
	if e == nil || e.Store == nil {
		return out
	}
	saveCtx := context.WithoutCancel(ctx)
	if latest, err := e.Store.Load(saveCtx, seed.RunID); err == nil {
		out = latest
		out.Status = core.RUN_STATUS_FAILED
		out.UpdatedAt = time.Now().UTC()
	}
	if err := e.Store.Save(saveCtx, out); err != nil {
		slog.Error("run_panic: failed to persist FAILED status", "run_id", seed.RunID, "err", err)
	}
	return out
}
