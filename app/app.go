package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/runtime"
)

// Exit codes. pm2 and any other supervisor read these to decide whether a
// tick errored, so they are part of the contract.
const (
	EXIT_OK    = 0
	EXIT_ERROR = 1
)

// Main is the entry point for an agent binary:
//
//	func main() { app.Main(&reviewAgent{}) }
//
// It binds SIGINT / SIGTERM to the run context so a supervisor's stop
// signal cancels the in-flight model call or tool instead of killing the
// process mid-write, then exits with Run's code.
func Main(a Agent, opts ...Option) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	os.Exit(Run(ctx, a, opts...))
}

// Run is the testable core of Main: the full lifecycle, returning an exit
// code rather than calling os.Exit.
//
// The sequence is fixed, and the order is the point — each step establishes
// what the next one may assume:
//
//  1. config   — open ~/.config/<name>, install the run logger
//  2. preflight— validate credentials and dependencies (optional)
//  3. deadline — bound total wall-clock time
//  4. bootstrap— the agent assembles its Engine and opening State
//  5. run      — drive the loop under panic recovery
//  6. complete — hand the final State back to the agent (optional)
func Run(ctx context.Context, a Agent, opts ...Option) int {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}

	name := a.Name()
	if name == "" {
		name = "agentSDK"
	}

	// 1. Config. Opens the app dirs, generates the run ID, wires the
	//    file-backed StateStore + WAL, and swaps slog's default handler.
	cfg, err := config.OpenForCLI(name, o.logLevel)
	if err != nil {
		slog.Error("config load failed", "app", name, "err", err)
		return EXIT_ERROR
	}
	if o.logToStdout {
		useStdoutLog(cfg.RunID, o.logLevel)
	}
	log := slog.Default().With(slog.String("app", name))

	// 2. Preflight. Fail before the run leaves any trace.
	if p, ok := a.(Preflighter); ok {
		if err := p.Preflight(ctx, cfg); err != nil {
			log.Error("preflight failed", "err", err)
			return EXIT_ERROR
		}
	}

	// 3. Deadline. A non-positive timeout opts out.
	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}

	// 4. Bootstrap. The agent owns composition; we only backfill the
	//    persistence and run ID it did not set for itself.
	engine, state, err := a.Bootstrap(ctx, cfg)
	if err != nil {
		log.Error("bootstrap failed", "err", err)
		return EXIT_ERROR
	}
	if engine == nil {
		log.Error("bootstrap returned a nil Engine")
		return EXIT_ERROR
	}
	if engine.Store == nil {
		engine.Store = cfg.StateStore
	}
	if engine.Log == nil {
		engine.Log = cfg.WAL
	}
	if state.RunID == "" {
		state.RunID = cfg.RunID
	}

	// 5. Run under panic recovery.
	start := time.Now()
	log.Info("run_start", "run_id", state.RunID)
	final, runErr := safeRun(ctx, engine, state)
	dur := time.Since(start)

	if runErr != nil {
		log.Error("run_failed",
			"run_id", state.RunID,
			"dur_ms", dur.Milliseconds(),
			"turns", final.Turn,
			"err", runErr)
		return EXIT_ERROR
	}

	// 6. Completion hook. A run whose results could not be delivered did
	//    not succeed, so an error here fails the process.
	if c, ok := a.(Completer); ok {
		if err := c.OnComplete(ctx, final); err != nil {
			log.Error("on_complete failed", "run_id", state.RunID, "err", err)
			return EXIT_ERROR
		}
	}

	log.Info("run_done",
		"run_id", state.RunID,
		"dur_ms", dur.Milliseconds(),
		"turns", final.Turn,
		"status", string(final.Status))
	return EXIT_OK
}

// safeRun drives the engine with panic recovery.
//
// A panic inside a tool would otherwise unwind straight through
// Engine.runStep, skipping the Store.Save that closes out the step — the
// run would be left persisted as `running` forever, and a later Resume
// would replay from that stale snapshot. So on recovery we reload whatever
// the engine last committed and mark it FAILED, making the crash visible
// and the run terminal.
//
// Recovery covers the calling goroutine only. An Agent that spawns its own
// goroutines must guard them itself.
func safeRun(ctx context.Context, e *runtime.Engine, state core.State) (final core.State, err error) {
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

// markFailed persists a terminal FAILED status for a crashed run. It reads
// back the engine's own last checkpoint rather than the seed state, so the
// recorded turn count and message history reflect how far the run actually
// got.
//
// It runs on a cancellation-detached context: the panic may well have been
// preceded by ctx expiring, and a store write that fails because of the
// very condition it is recording would defeat the purpose.
func markFailed(ctx context.Context, e *runtime.Engine, seed core.State) core.State {
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

// useStdoutLog replaces the file handler config.OpenForCLI installed with a
// JSON handler on stdout, keeping the run_id attribute.
func useStdoutLog(runID string, level slog.Level) {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h).With(slog.String("run_id", runID)))
}
