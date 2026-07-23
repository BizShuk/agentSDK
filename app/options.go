package app

import (
	"log/slog"
	"time"
)

// DEFAULT_RUN_TIMEOUT caps total wall-clock time for one process. Generous
// enough for any reasonable agentic loop; an agent that needs a tighter
// bound should set core.Budget.MaxWallTime in Bootstrap, which the engine
// checks between steps, or pass WithTimeout.
//
// This is a hard ctx deadline, not an advisory budget: it cuts a hung
// provider call that the per-instruction timeout somehow survived.
const DEFAULT_RUN_TIMEOUT = 30 * time.Minute

// DEFAULT_ROUND_TIMEOUT caps how long a single Interactive.NextRound call
// may block. Generous because an operator-in-the-loop decision can take
// minutes; a non-positive value disables the per-round deadline, leaving
// only WithTimeout's run-wide bound.
const DEFAULT_ROUND_TIMEOUT = 30 * time.Minute

// options carries the tunables Main/Run apply around the Agent.
type options struct {
	timeout      time.Duration
	roundTimeout time.Duration
	logLevel     slog.Level
	logToStdout  bool
}

// Option customizes Run. All have defaults; none are required.
type Option func(*options)

func defaultOptions() options {
	return options{
		timeout:      DEFAULT_RUN_TIMEOUT,
		roundTimeout: DEFAULT_ROUND_TIMEOUT,
		logLevel:     slog.LevelInfo,
	}
}

// WithTimeout overrides DEFAULT_RUN_TIMEOUT. A non-positive duration
// disables the deadline entirely — the run is then bounded only by
// Budget and by signals.
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithLogLevel sets the slog level for the run log. Default slog.LevelInfo.
func WithLogLevel(l slog.Level) Option {
	return func(o *options) { o.logLevel = l }
}

// WithLogToStdout redirects the run log to stdout instead of the per-run
// file under ~/.config/<app>/log/.
//
// Required under a process supervisor: pm2 captures a process's stdout into
// its own log store, so an agent that logs only to a file is invisible to
// `pm2 logs`. Default is the file handler that config.OpenForCLI installs.
func WithLogToStdout() Option {
	return func(o *options) { o.logToStdout = true }
}

// WithRoundTimeout bounds a single Interactive.NextRound call. Each pause
// gets a fresh deadline, so this caps per-answer latency, not the run's
// total interactive time — that is WithTimeout's job.
//
// A non-positive duration disables the per-round deadline, leaving
// NextRound bounded only by WithTimeout and by signals.
func WithRoundTimeout(d time.Duration) Option {
	return func(o *options) { o.roundTimeout = d }
}
