package agent

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware/hook"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/runtime"
)

// middlewareChain names the chain type so build.go can return nil for the
// "none" preset without importing middleware for a single reference.
type middlewareChain = middleware.Middleware

// Option injects what a config file cannot express: a live provider, a
// closure hook, a custom tool, an event sink.
//
// This is the repo's standard shape for dependency injection — app and
// every provider adapter use `type Option func(*x)`. The one difference
// is the error return: an injection can legitimately fail (a duplicate
// tool name, a rule whose Kind does not match what the config enabled),
// and failing at New beats failing halfway through assembly.
//
// The full set of With* functions lives in this file on purpose. That
// listing IS the answer to "what does an application have to inject",
// which a struct of fields would answer no better and godoc answers here
// for free.
type Option func(*builder) error

// builder accumulates injected dependencies before assembly reads them.
// It is unexported: callers configure it through Option only, so adding a
// field later cannot break anyone.
type builder struct {
	provider  core.Provider
	sink      core.EventSink
	listener  core.ObservationSource
	notifier  core.Notifier
	tools     []core.Tool
	hooks     []hook.Rule
	sources   []prompt.Source
	rules     []core.DecisionRule
	customize func(*runtime.Engine) error
}

// hookRunner builds the core.Hooks implementation from the injected
// rules. Called only when there is at least one, so a run with no hooks
// keeps Engine.Hooks nil and pays nothing.
func (b *builder) hookRunner() core.Hooks {
	return hook.NewRunner(b.hooks...)
}

// apply runs the options in order, stopping at the first failure.
func (b *builder) apply(opts []Option) error {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(b); err != nil {
			return fmt.Errorf("agent: option: %w", err)
		}
	}
	return nil
}

// WithProvider supplies the model provider directly, overriding the
// Model block entirely. This is how a test injects a fake, and how an
// application reuses a provider it already built (a shared client, or one
// wrapped by config.NewRefreshingProvider).
func WithProvider(p core.Provider) Option {
	return func(b *builder) error {
		if p == nil {
			return fmt.Errorf("WithProvider: provider must not be nil")
		}
		b.provider = p
		return nil
	}
}

// WithSink binds the presentation stream. It overrides whatever the
// Output block selected, for callers that render events themselves
// instead of using the built-in text/json/tui formats.
func WithSink(s core.EventSink) Option {
	return func(b *builder) error {
		if s == nil {
			return fmt.Errorf("WithSink: sink must not be nil")
		}
		b.sink = s
		return nil
	}
}

// WithNotifier binds the out-of-band notification port.
func WithNotifier(n core.Notifier) Option {
	return func(b *builder) error {
		if n == nil {
			return fmt.Errorf("WithNotifier: notifier must not be nil")
		}
		b.notifier = n
		return nil
	}
}

// WithListener binds a perception source. Bootstrap spawns a goroutine
// that reads the source's Observations channel and forwards each
// observation's Payload to Engine.Steer, so the next Decide cycle sees
// it as a user message. The goroutine terminates when the listener's
// channel closes OR when Bootstrap's context is cancelled — whichever
// comes first.
//
// Use this for long-running streams: log tails, journal watchers, sensor
// feeds. For one-shot input (stdin, a CLI flag) keep reading in your
// Agent implementation and append to state.Messages directly; Steer
// assumes the engine is already inside Engine.Run.
func WithListener(src core.ObservationSource) Option {
	return func(b *builder) error {
		if src == nil {
			return fmt.Errorf("WithListener: source must not be nil")
		}
		b.listener = src
		return nil
	}
}

// SinkFunc adapts a plain callback to core.EventSink, for callers that
// want events without declaring a type.
type SinkFunc func(core.StreamEvent)

// OnStreamEvent implements core.EventSink.
func (f SinkFunc) OnStreamEvent(ev core.StreamEvent) { f(ev) }

// WithTools adds application-specific tools alongside the built-ins the
// config enabled. Registration order decides a name clash, so a tool
// added here overrides a built-in of the same name.
func WithTools(tools ...core.Tool) Option {
	return func(b *builder) error {
		for i, t := range tools {
			if t == nil {
				return fmt.Errorf("WithTools: tool %d is nil", i)
			}
		}
		b.tools = append(b.tools, tools...)
		return nil
	}
}

// WithHooks adds lifecycle hook rules — the closure-based safety gates
// and side effects that a config file cannot express.
//
// A PreToolUse hook that blocks folds into a failed ToolResult, so the
// model sees the refusal and can react, rather than the call silently
// vanishing.
func WithHooks(rules ...hook.Rule) Option {
	return func(b *builder) error {
		b.hooks = append(b.hooks, rules...)
		return nil
	}
}

// WithSources adds prompt content sources beyond the ones the Prompt
// block names. They are appended after the configured sources, but
// placement is decided by each Section's Order, not by this position.
func WithSources(sources ...prompt.Source) Option {
	return func(b *builder) error {
		for i, s := range sources {
			if s == nil {
				return fmt.Errorf("WithSources: source %d is nil", i)
			}
		}
		b.sources = append(b.sources, sources...)
		return nil
	}
}

// WithRules registers reasoning strategies beyond the built-in six, or
// replaces one of them: a rule whose Kind matches a configured style wins
// over the built-in implementation.
func WithRules(rules ...core.DecisionRule) Option {
	return func(b *builder) error {
		for i, r := range rules {
			if r == nil {
				return fmt.Errorf("WithRules: rule %d is nil", i)
			}
		}
		b.rules = append(b.rules, rules...)
		return nil
	}
}

// WithCustomize runs last, after every stage, with the fully assembled
// engine. It is the escape hatch for anything the config vocabulary does
// not cover — and the reason this layer is a preset, not a wall.
func WithCustomize(fn func(*runtime.Engine) error) Option {
	return func(b *builder) error {
		if fn == nil {
			return fmt.Errorf("WithCustomize: function must not be nil")
		}
		b.customize = fn
		return nil
	}
}

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

// runOpts carries the tunables Main/Run apply around the Runner. It is
// separate from builder because the lifecycle tunables are runtime-shape
// concerns (timeouts, log destination), while builder is assembly-shape
// (provider, tools, hooks). Splitting them keeps the two contracts —
// Option (may fail) and RunOption (cannot fail) — distinct.
type runOpts struct {
	timeout      time.Duration
	roundTimeout time.Duration
	logLevel     slog.Level
	logToStdout  bool
}

// RunOption customizes Main/Run. All have defaults; none are required.
//
// The type is distinct from Option (which assembles a *Agent and may fail)
// because the lifecycle is shape-only: changing a timeout or log level
// cannot fail, so a RunOption has no error return.
type RunOption func(*runOpts)

func defaultRunOpts() runOpts {
	return runOpts{
		timeout:      DEFAULT_RUN_TIMEOUT,
		roundTimeout: DEFAULT_ROUND_TIMEOUT,
		logLevel:     slog.LevelInfo,
	}
}

// WithTimeout overrides DEFAULT_RUN_TIMEOUT. A non-positive duration
// disables the deadline entirely — the run is then bounded only by
// Budget and by signals.
func WithTimeout(d time.Duration) RunOption {
	return func(o *runOpts) { o.timeout = d }
}

// WithLogLevel sets the slog level for the run log. Default slog.LevelInfo.
func WithLogLevel(l slog.Level) RunOption {
	return func(o *runOpts) { o.logLevel = l }
}

// WithLogToStdout redirects the run log to stdout instead of the per-run
// file under ~/.config/<app>/log/.
//
// Required under a process supervisor: pm2 captures a process's stdout into
// its own log store, so an agent that logs only to a file is invisible to
// `pm2 logs`. Default is the file handler that config.OpenForCLI installs.
func WithLogToStdout() RunOption {
	return func(o *runOpts) { o.logToStdout = true }
}

// WithRoundTimeout bounds a single Interactive.NextRound call. Each pause
// gets a fresh deadline, so this caps per-answer latency, not the run's
// total interactive time — that is WithTimeout's job.
//
// A non-positive duration disables the per-round deadline, leaving
// NextRound bounded only by WithTimeout and by signals.
func WithRoundTimeout(d time.Duration) RunOption {
	return func(o *runOpts) { o.roundTimeout = d }
}
