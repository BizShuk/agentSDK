// Package cli is the process-hosting side of the agent layer. Where
// agent/ stays embeddable (no globals, no signal handling), cli is the
// entry point a CLI binary calls: it creates the runtime directories,
// installs slog's default handler, binds signal handlers to the run
// context, and exits with the right code.
//
// A caller that wants the agent without surrendering those knobs
// (an HTTP server, a test harness, an in-process library) should build
// a Host via agent.Open and hand it to agent.Run directly.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bizshuk/agentsdk/agent"
)

// Exit codes. pm2 and any other supervisor read these to decide whether
// a tick errored, so they are part of the contract.
const (
	EXIT_OK    = 0
	EXIT_ERROR = 1
)

// OpenForCLI builds a Host for a CLI binary: runs agent.Open, mkdirs the
// runtime directories (<dataDir>/states, <dataDir>/wal, <logDir>),
// generates the run ID, opens <logDir>/<runID>.log, and swaps slog's
// default handler to a JSON file handler at the given level.
//
// Returns an error when appName is empty. Pair with cli.Run for the
// full lifecycle.
func OpenForCLI(appName string, level slog.Level) (*agent.Host, error) {
	host, err := agent.Open(appName)
	if err != nil {
		return nil, err
	}
	statesDir := filepath.Join(host.DataDir, "states")
	walDir := filepath.Join(host.DataDir, "wal")
	for _, d := range []string{statesDir, walDir, host.LogDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, fmt.Errorf("cli: mkdir %s: %w", d, err)
		}
	}

	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	logFile := filepath.Join(host.LogDir, runID+".log")
	if err := openRunLog(logFile, runID, level); err != nil {
		return nil, fmt.Errorf("cli: open log: %w", err)
	}
	host.RunID = runID
	host.LogFile = logFile
	host.Logger = slog.Default()
	return host, nil
}

// MustOpenForCLI is like OpenForCLI but panics on error — for CLI entry
// points where failure is always a programmer error (e.g. empty appName).
func MustOpenForCLI(appName string, level slog.Level) *agent.Host {
	host, err := OpenForCLI(appName, level)
	if err != nil {
		panic(err)
	}
	return host
}

// Main is the entry point for an agent binary:
//
//	func main() { cli.Main(agent.MustNew(cfg)) }
//
// It binds SIGINT / SIGTERM to the run context, drives the lifecycle via
// cli.Run, and exits with the code Run returns. Signal handling lets a
// supervisor's stop signal cancel the in-flight model call or tool
// instead of killing the process mid-write.
func Main(a agent.Runner, opts ...agent.RunOption) {
	if a == nil {
		slog.Error("cli: runner is required")
		os.Exit(EXIT_ERROR)
	}
	name := a.Name()
	if name == "" {
		slog.Error("cli: Runner.Name must not be empty")
		os.Exit(EXIT_ERROR)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	os.Exit(Run(ctx, a, opts...))
}

// Run is the testable core of Main: it builds a Host via OpenForCLI,
// hands off to agent.Run, then maps the returned error to a process exit
// code. A caller that wants to embed the agent without exiting the process
// should call agent.Run directly with a Host of its own.
func Run(ctx context.Context, a agent.Runner, opts ...agent.RunOption) int {
	if a == nil {
		slog.Error("cli: runner is required")
		return EXIT_ERROR
	}
	o := agent.DefaultRunOpts()
	for _, opt := range opts {
		opt(&o)
	}
	name := a.Name()
	host, err := OpenForCLI(name, o.LogLevel)
	if err != nil {
		slog.Error("config load failed", "app", name, "err", err)
		return EXIT_ERROR
	}
	if o.LogToStdout {
		useStdoutLog(host.RunID, o.LogLevel)
	}
	if err := agent.Run(ctx, a, host, opts...); err != nil {
		slog.Error("run failed", "app", name, "err", err)
		return EXIT_ERROR
	}
	return EXIT_OK
}

// useStdoutLog replaces the file handler OpenForCLI installed with a
// JSON handler on stdout, keeping the run_id attribute.
func useStdoutLog(runID string, level slog.Level) {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h).With(slog.String("run_id", runID)))
}

// openRunLog swaps slog default to a JSON file handler writing to logFile,
// and attaches runID to every record for grep-ability.
func openRunLog(logFile, runID string, level slog.Level) error {
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h).With(slog.String("run_id", runID)))
	return nil
}
