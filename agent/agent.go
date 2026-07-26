// Package agent turns declarative configuration into a runnable agent.
// Use Once for a single model call, or New with cli.Main for a full agent.
package agent

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/agent/session"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/skill"
)

// Configuration aliases keep the common API under one import while spec
// remains independently usable by config tooling.
type (
	Config     = spec.Config
	Model      = spec.Model
	Reasoning  = spec.Reasoning
	Limits     = spec.Limits
	Middleware = spec.Middleware
	Memory     = spec.Memory
	Tools      = spec.Tools
	Safety     = spec.Safety
	Prompt     = spec.Prompt
	Skills     = spec.Skills
	Subagents  = spec.Subagents
	Sessions   = spec.Sessions
	Output     = spec.Output

	// Engine hides the runtime package from application signatures.
	Engine = runtime.Engine
)

// Agent is a prepared configuration and its injected dependencies.
type Agent struct {
	cfg   Config
	deps  builder
	parts *Parts
}

// Parts exposes the components produced by Bootstrap. It is nil before
// Bootstrap runs.
type Parts struct {
	Engine    *Engine
	Sessions  *session.Manager
	Skills    *skill.Registry
	Prompt    prompt.Builder
	Config    Config
	AppConfig *AppConfig
	Cwd       string
}

// New validates and prepares an agent without performing I/O.
func New(cfg Config, opts ...Option) (*Agent, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("agent: name is required (use Once for a nameless single call)")
	}
	prepared, err := cfg.Prepare()
	if err != nil {
		return nil, err
	}
	var deps builder
	if err := deps.apply(opts); err != nil {
		return nil, err
	}
	return &Agent{cfg: prepared, deps: deps}, nil
}

// MustNew is New for entry points where invalid configuration is a
// programming error.
func MustNew(cfg Config, opts ...Option) *Agent {
	a, err := New(cfg, opts...)
	if err != nil {
		panic(err)
	}
	return a
}

// Name implements Runner.
func (a *Agent) Name() string { return a.cfg.Name }

// Config returns the expanded, validated configuration.
func (a *Agent) Config() Config { return a.cfg }

// Parts returns the last Bootstrap result.
func (a *Agent) Parts() *Parts { return a.parts }

// Preflight validates provider construction before creating run state.
func (a *Agent) Preflight(_ context.Context, _ *Host) error {
	_, err := a.deps.buildProvider(a.cfg.Model)
	return err
}

// Runner supplies an engine and opening state to Run.
type Runner interface {
	Name() string
	Bootstrap(ctx context.Context, host *Host) (*Engine, core.State, error)
}

// Preflighter performs optional checks before Bootstrap.
type Preflighter interface {
	Preflight(ctx context.Context, host *Host) error
}

// Completer receives a successfully completed run.
type Completer interface {
	OnComplete(ctx context.Context, final core.State) error
}

// PauseReason identifies why an interactive run stopped.
type PauseReason string

const (
	PAUSE_APPROVAL  PauseReason = "approval"
	PAUSE_ROUND_END PauseReason = "round_end"
)

// Pause describes the stopped run handed to Interactive.
type Pause struct {
	State  core.State
	Reason PauseReason
}

// Resume describes how an Interactive implementation continues a run.
type Resume struct {
	Decision core.ApprovalDecision
	Input    string
	Stop     bool
	By       string
}

// Interactive handles approvals and follow-up input between rounds.
// Implementations must honor context cancellation.
type Interactive interface {
	NextRound(ctx context.Context, pause Pause) (Resume, error)
}
