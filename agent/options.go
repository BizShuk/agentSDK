package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware/hook"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/tool"
)

// Option injects live dependencies that configuration cannot express.
type Option func(*builder) error

type builder struct {
	provider       core.Provider
	sink           core.EventSink
	listener       core.ObservationSource
	notifier       core.Notifier
	tools          []core.Tool
	toolRegistrars []func(*tool.Registry)
	hooks          []hook.Rule
	sources        []prompt.Source
	rules          []core.DecisionRule
	customize      func(*Engine) error
}

func (b *builder) hookRunner() core.Hooks {
	return hook.NewRunner(b.hooks...)
}

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

func (b *builder) buildProvider(model Model) (core.Provider, error) {
	if b.provider != nil {
		return b.provider, nil
	}
	return provider.New(model.Provider, provider.Options{
		Model:          model.Name,
		BaseURL:        model.BaseURL,
		APIKeyEnv:      model.APIKeyEnv,
		CredentialKind: model.CredentialKind,
	})
}

// WithProvider overrides the configured model provider.
func WithProvider(p core.Provider) Option {
	return func(b *builder) error {
		if p == nil {
			return fmt.Errorf("WithProvider: provider must not be nil")
		}
		b.provider = p
		return nil
	}
}

// WithSink overrides the configured event sink.
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

// WithListener forwards observations to Engine.Steer until the source
// closes or Bootstrap's context is cancelled.
func WithListener(src core.ObservationSource) Option {
	return func(b *builder) error {
		if src == nil {
			return fmt.Errorf("WithListener: source must not be nil")
		}
		b.listener = src
		return nil
	}
}

// SinkFunc adapts a callback to core.EventSink.
type SinkFunc func(core.StreamEvent)

// OnStreamEvent implements core.EventSink.
func (f SinkFunc) OnStreamEvent(ev core.StreamEvent) { f(ev) }

// WithTools adds pre-built tools after configured built-ins.
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

// WithToolRegistrar adds custom registry functions after configured built-ins.
func WithToolRegistrar(fns ...func(*tool.Registry)) Option {
	return func(b *builder) error {
		for i, fn := range fns {
			if fn == nil {
				return fmt.Errorf("WithToolRegistrar: registrar %d is nil", i)
			}
		}
		b.toolRegistrars = append(b.toolRegistrars, fns...)
		return nil
	}
}

// WithToolFunc registers a typed Go function directly as a tool.
func WithToolFunc[TArgs any, TOut any](name, desc string, risk core.RiskLevel, fn func(ctx context.Context, args TArgs) (TOut, error)) Option {
	if fn == nil {
		return func(*builder) error {
			return fmt.Errorf("WithToolFunc: function must not be nil")
		}
	}
	return WithToolRegistrar(func(reg *tool.Registry) {
		tool.RegisterFunc(reg, name, desc, risk, fn)
	})
}

// WithHooks adds lifecycle hook rules.
func WithHooks(rules ...hook.Rule) Option {
	return func(b *builder) error {
		b.hooks = append(b.hooks, rules...)
		return nil
	}
}

// WithSources adds prompt sources after configured sources.
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

// WithRules adds or replaces reasoning strategies by Kind.
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

// WithCustomize runs last with the fully assembled engine.
func WithCustomize(fn func(*Engine) error) Option {
	return func(b *builder) error {
		if fn == nil {
			return fmt.Errorf("WithCustomize: function must not be nil")
		}
		b.customize = fn
		return nil
	}
}

// DEFAULT_RUN_TIMEOUT is the default process deadline.
const DEFAULT_RUN_TIMEOUT = 30 * time.Minute

// DEFAULT_ROUND_TIMEOUT is the default deadline for one interactive pause.
const DEFAULT_ROUND_TIMEOUT = 30 * time.Minute

// RunOpts contains lifecycle and process-log settings.
type RunOpts struct {
	Timeout      time.Duration
	RoundTimeout time.Duration
	LogLevel     slog.Level
	LogToStdout  bool
}

// DefaultRunOpts returns the default lifecycle settings.
func DefaultRunOpts() RunOpts {
	return RunOpts{
		Timeout:      DEFAULT_RUN_TIMEOUT,
		RoundTimeout: DEFAULT_ROUND_TIMEOUT,
		LogLevel:     slog.LevelInfo,
	}
}

// RunOption customizes Run and cli.Main.
type RunOption func(*RunOpts)

// WithTimeout sets the process deadline; a non-positive value disables it.
func WithTimeout(d time.Duration) RunOption {
	return func(o *RunOpts) { o.Timeout = d }
}

// WithLogLevel sets the slog level for the run log. Default slog.LevelInfo.
func WithLogLevel(l slog.Level) RunOption {
	return func(o *RunOpts) { o.LogLevel = l }
}

// WithLogToStdout sends the run log to stdout instead of its per-run file.
func WithLogToStdout() RunOption {
	return func(o *RunOpts) { o.LogToStdout = true }
}

// WithRoundTimeout sets the deadline per Interactive.NextRound call.
func WithRoundTimeout(d time.Duration) RunOption {
	return func(o *RunOpts) { o.RoundTimeout = d }
}
