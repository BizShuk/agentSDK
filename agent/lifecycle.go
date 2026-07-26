package agent

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/bizshuk/agentsdk/core"
)

// Exit codes. pm2 and any other supervisor read these to decide whether a
// tick errored, so they are part of the contract.
//
// Deprecated: exit-code constants moved to agent/cli. The values are
// unchanged (0 / 1); new code should import cli.EXIT_OK / cli.EXIT_ERROR.
const (
	EXIT_OK    = 0
	EXIT_ERROR = 1
)

// Run is the testable core of cli.Main: the full lifecycle, returning
// an exit code rather than calling os.Exit. The host is provided
// pre-built (by cli.OpenForCLI, by a test fixture, or by any caller that
// wants to embed the agent).
//
// The sequence is fixed, and the order is the point — each step
// establishes what the next one may assume:
//
//  1. ctx check — already-dead contexts return immediately
//  2. preflight— validate credentials and dependencies (optional)
//  3. deadline — bound total wall-clock time
//  4. bootstrap— the agent assembles its Engine and opening State
//  5. run      — drive the loop under panic recovery
//  6. complete — hand the final State back to the agent (optional)
func Run(ctx context.Context, a Runner, host *Host, opts ...RunOption) int {
	o := defaultRunOpts()
	for _, opt := range opts {
		opt(&o)
	}

	log := host.Logger
	if log == nil {
		log = slog.Default().With(slog.String("app", a.Name()))
	}

	// Empty name was historically a setup error: state and WAL would
	// resolve under an empty path. cli.Open already rejects this with a
	// clearer message, so we surface the error pre-Bootstrap to keep
	// the contract tested.
	if a.Name() == "" {
		log.Error("run: Runner.Name must not be empty")
		return EXIT_ERROR
	}

	// 1. Ctx check. A deadline that already expired gives the caller a
	//    failing exit without invoking Bootstrap.
	if err := ctx.Err(); err != nil {
		log.Error("run context cancelled before bootstrap", "err", err)
		return EXIT_ERROR
	}

	// 2. Preflight. Fail before the run leaves any trace.
	if p, ok := a.(Preflighter); ok {
		if err := p.Preflight(ctx, host); err != nil {
			log.Error("preflight failed", "err", err)
			return EXIT_ERROR
		}
	}

	// 3. Deadline. A non-positive timeout opts out.
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}

	// 4. Bootstrap. The agent owns composition; we only backfill the
	//    persistence and run ID it did not set for itself.
	engine, state, err := a.Bootstrap(ctx, host)
	if err != nil {
		log.Error("bootstrap failed", "err", err)
		return EXIT_ERROR
	}
	if engine == nil {
		log.Error("bootstrap returned a nil Engine")
		return EXIT_ERROR
	}
	if engine.Store == nil {
		engine.Store = host.StateStore
	}
	if engine.Log == nil {
		engine.Log = host.WAL
	}
	if state.RunID == "" {
		state.RunID = host.RunID
	}

	// 5. Run under panic recovery.
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

	// 5a. Interactive rounds. A run that stopped is asking the application
	//     a question — approve this call, or give me the next input. A
	//     Runner that does not implement Interactive skips this entirely
	//     and keeps the out-of-process semantics (Run returns,
	//     PendingApprovals left for an external verb).
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

	// 6. Completion hook. A run whose results could not be delivered did
	//    not succeed, so an error here fails the process.
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

// pauseReason classifies a stop into a question for the application, or
// reports that there is nothing to ask. FAILED and ABORTED return false:
// they are terminal, and asking the application to continue a failed run
// would paper over the failure.
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

// resolveRound bounds one NextRound call. cancel is deferred inside THIS
// function, which returns per call — a defer placed in the caller's loop
// would hold every pause's timer alive until Run itself returned.
func resolveRound(ctx context.Context, in Interactive, d time.Duration, p Pause) (Resume, error) {
	if d <= 0 {
		return in.NextRound(ctx, p)
	}
	rctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return in.NextRound(rctx, p)
}

// advance applies one Resume and drives the engine to its next stop.
//
// Two non-obvious mechanics:
//
// SubmitHumanDecision ALREADY re-enters runStep and drives the run forward
// (see runtime/loop.go). Calling Resume afterwards would load the persisted
// state, find no undecided approval, fall through to WAL replay, and
// re-execute every logged event — duplicate tool and model calls. So it is
// deliberately NOT called here.
//
// Steer, not FollowUp, carries the new input. FollowUp only fires from
// inside runStep when the loop would otherwise complete; by the time Run
// sees a status the engine has already returned, so the queue would never
// be read. Steer is drained at the top of the next Decide, which is exactly
// where a new user message belongs.
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
			// No answer is not consent for a call the policy flagged.
			decision = core.APPROVAL_DECISION_REJECT
		}
		return e.SubmitHumanDecision(ctx, s.RunID, decision, by)
	}
	// PAUSE_ROUND_END: runStep short-circuits on a COMPLETED status, so the
	// run has to be reopened before the steered message can be seen. Reset
	// the FSM phase so it re-enters reasoning instead of emitting DONE on
	// stale dispatch memory (the same seam the follow-up queue uses).
	next := s.Clone()
	next.Status = core.RUN_STATUS_RUNNING
	delete(next.WorkingMemory, "think_then_act.phase")
	return e.Run(ctx, next)
}

// safeRun drives the engine with panic recovery.
//
// A panic inside a tool would otherwise unwind straight through
// Engine.runStep, skipping the Store.Save that closes out the step — the
// run would be left persisted as `running` forever, and a later Resume
// would replay from that stale snapshot. So on recovery we reload
// whatever the engine last committed and mark it FAILED, making the crash
// visible and the run terminal.
//
// Recovery covers the calling goroutine only. A Runner that spawns its
// own goroutines must guard them itself.
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

// markFailed persists a terminal FAILED status for a crashed run. It
// reads back the engine's own last checkpoint rather than the seed state,
// so the recorded turn count and message history reflect how far the run
// actually got.
//
// It runs on a cancellation-detached context: the panic may well have
// been preceded by ctx expiring, and a store write that fails because of
// the very condition it is recording would defeat the purpose.
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
