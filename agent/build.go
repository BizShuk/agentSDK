package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bizshuk/agentsdk/agent/permission"
	"github.com/bizshuk/agentsdk/agent/session"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/preset"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/reasoning"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/skill"
	"github.com/bizshuk/agentsdk/tool"
	"github.com/bizshuk/agentsdk/tool/builtin"
)

var (
	errListenerClosed     = errors.New("observation source closed before its first non-empty observation")
	errNilObservationChan = errors.New("observation source returned a nil channel")
)

// NewEngine constructs the runtime container without exposing runtime in
// application signatures.
func NewEngine(decide core.Decide, provider core.Provider, tools core.ToolRegistry) *Engine {
	return runtime.NewEngine(decide, provider, tools)
}

// ReActStep returns the default think-then-act dispatcher.
func ReActStep() core.Decide {
	return reasoning.NewDecide(map[string]reasoning.DecisionRule{
		core.REASON_REACT: reasoning.NewThinkThenAct(),
	})
}

// Bootstrap assembles the configured pipeline and opening state. Ordering
// matters: tools may need the provider, prompts may need skills, and one
// permission engine must feed both middleware and Engine.Approval.
func (a *Agent) Bootstrap(ctx context.Context, ac *Host) (*Engine, core.State, error) {
	if ac == nil {
		return nil, core.State{}, errNoHost
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, core.State{}, fmt.Errorf("agent: getwd: %w", err)
	}
	userDir := filepath.Dir(ac.DataDir)

	// --- stage 1: provider ---
	prov, err := a.deps.buildProvider(a.cfg.Model)
	if err != nil {
		return nil, core.State{}, err
	}

	// --- stage 2: tools ---
	tools, err := a.buildTools(cwd, userDir, prov)
	if err != nil {
		return nil, core.State{}, err
	}

	// --- stage 3: reasoning ---
	step, err := a.buildDecide()
	if err != nil {
		return nil, core.State{}, err
	}

	// --- stage 4: prompt ---
	skills, err := discoverSkills(a.cfg, userDir, cwd)
	if err != nil {
		return nil, core.State{}, err
	}
	sources, err := BuildSources(a.cfg, skills, userDir)
	if err != nil {
		return nil, core.State{}, err
	}
	sources = append(sources, a.deps.sources...)
	builderP := prompt.Builder{Sources: sources, MaxBytes: a.promptMaxBytes()}

	// --- stage 5: safety ---
	perm, sandbox := a.buildSafety()

	// --- stage 6: memory ---
	store, wal := a.buildPersistence(ac)
	sessions, err := a.buildSessions(ac, store, wal)
	if err != nil {
		return nil, core.State{}, err
	}

	// --- stage 7: assemble ---
	eng := NewEngine(step, prov, tools)
	eng.Middleware = a.buildMiddleware(sandbox, perm)
	eng.Store = store
	eng.Log = wal
	eng.Sink = a.deps.sink
	eng.Notifier = a.deps.notifier
	if perm != nil {
		eng.Approval = perm
	}
	if len(a.deps.hooks) > 0 {
		eng.Hooks = a.deps.hookRunner()
	}

	state, err := a.seedState(ctx, ac, builderP, cwd)
	if err != nil {
		return nil, core.State{}, err
	}

	if a.deps.customize != nil {
		if err := a.deps.customize(eng); err != nil {
			return nil, core.State{}, fmt.Errorf("agent: customize: %w", err)
		}
	}

	// Bind listeners only after the engine is fully wired. The first
	// observation is queued synchronously so the opening model request
	// cannot race ahead with only the seeded prompt.
	if a.deps.listener != nil {
		if err := startListener(ctx, eng, a.deps.listener); err != nil {
			return nil, core.State{}, fmt.Errorf("agent: listener: %w", err)
		}
	}

	a.parts = &Parts{
		Engine: eng, Sessions: sessions, Skills: skills, Host: ac, Cwd: cwd,
	}
	return eng, state, nil
}

// --- stage 2 ---

// buildTools registers built-ins, injected tools, then subagents. It returns
// nil when no tools are configured.
func (a *Agent) buildTools(cwd, userDir string, prov core.Provider) (core.ToolRegistry, error) {
	if a.cfg.Tools == nil && a.cfg.Subagents == nil && len(a.deps.tools) == 0 && len(a.deps.toolRegistrars) == 0 {
		return nil, nil
	}
	reg := tool.NewRegistry()

	if a.cfg.Tools != nil {
		workDir := a.cfg.Tools.WorkingDir
		if workDir == "" {
			workDir = cwd
		}
		if err := builtin.Register(reg, a.cfg.Tools.Builtin, builtin.Options{
			Policy:     tool.DefaultPolicy(),
			WorkingDir: workDir,
		}); err != nil {
			return nil, fmt.Errorf("agent: register built-in tools: %w", err)
		}
	}

	for _, t := range a.deps.tools {
		reg.Register(t)
	}
	for _, fn := range a.deps.toolRegistrars {
		fn(reg)
	}

	if a.cfg.Subagents != nil {
		defs, err := discoverSubagentDefs(a.cfg, userDir, cwd)
		if err != nil {
			return nil, err
		}
		if len(defs) > 0 {
			run, err := a.subagentRunner(prov)
			if err != nil {
				return nil, err
			}
			spawner := skill.NewSpawner(run, defs...)
			spawner.MaxDepth = a.cfg.Subagents.MaxDepth
			reg.Register(spawner)
		}
	}
	return reg, nil
}

// discoverSubagentDefs lets project definitions override user definitions.
func discoverSubagentDefs(cfg Config, userDir, cwd string) ([]skill.SubAgent, error) {
	dirs := cfg.Subagents.Dirs
	if len(dirs) == 0 {
		dirs = discoveryRoots(cfg, userDir, cwd, "agents")
	}
	reg := skill.NewRegistry()
	for _, dir := range dirs {
		if err := reg.DiscoverSubagents(dir); err != nil {
			return nil, fmt.Errorf("discover subagents in %s: %w", dir, err)
		}
	}
	return reg.Subagents(), nil
}

// subagentRunner uses an ephemeral engine with the shared provider and its
// own turn budget.
func (a *Agent) subagentRunner(prov core.Provider) (skill.RunFunc, error) {
	maxTurns := a.cfg.Subagents.MaxTurns
	autonomy, err := core.ParseAutonomyLevel(a.cfg.Limits.Autonomy)
	if err != nil {
		return nil, fmt.Errorf("agent: subagent autonomy: %w", err)
	}
	return func(ctx context.Context, def skill.SubAgent, promptText string) (string, error) {
		reg := tool.NewRegistry()
		if a.cfg.Tools != nil {
			workDir := a.cfg.Tools.WorkingDir
			if workDir == "" {
				workDir = "."
			}
			// A subagent gets the tools its definition names, or the
			// parent's set when it names none.
			allow := def.Tools
			if len(allow) == 0 {
				allow = a.cfg.Tools.Builtin
			}
			if err := builtin.Register(reg, allow, builtin.Options{
				Policy:     tool.DefaultPolicy(),
				WorkingDir: workDir,
			}); err != nil {
				return "", fmt.Errorf("agent: register subagent built-in tools: %w", err)
			}
		}
		step := reasoning.NewDecide(map[string]reasoning.DecisionRule{
			core.REASON_REACT: reasoning.NewThinkThenAct(),
		})
		sub := NewEngine(step, prov, reg)
		perm, sandbox := a.buildSafety()
		sub.Middleware = a.buildMiddleware(sandbox, perm)
		if perm != nil {
			sub.Approval = perm
		}

		st := core.State{
			RunID:          fmt.Sprintf("sub-%d", time.Now().UnixNano()),
			ReasoningStyle: core.REASON_REACT,
			Autonomy:       autonomy,
			Budget:         core.Budget{MaxTurns: maxTurns},
		}
		if def.Prompt != "" {
			st.Messages = append(st.Messages, message(core.ROLE_SYSTEM, def.Prompt))
		}
		st.Messages = append(st.Messages, message(core.ROLE_USER, promptText))

		final, err := sub.Run(skill.WithDepth(ctx, skill.Depth(ctx)+1), st)
		if err != nil {
			return "", err
		}
		return LastAssistantText(final), nil
	}, nil
}

// --- stage 3 ---

// buildDecide registers enabled rules; injected rules override by Kind.
func (a *Agent) buildDecide() (core.Decide, error) {
	rules := map[string]reasoning.DecisionRule{}
	for _, name := range a.cfg.Reasoning.Enable {
		rule, err := reasoning.NewRule(name)
		if err != nil {
			return nil, fmt.Errorf("agent: build reasoning: %w", err)
		}
		rules[rule.Kind()] = rule
	}
	for _, rule := range a.deps.rules {
		rules[rule.Kind()] = rule
	}
	return reasoning.NewDecide(rules), nil
}

// --- stage 5 ---

// buildSafety creates the shared approval engine and optional sandbox.
func (a *Agent) buildSafety() (*permission.Engine, tool.Sandbox) {
	if a.cfg.Safety == nil {
		return nil, nil
	}
	rules := make([]permission.Rule, 0,
		len(a.cfg.Safety.Deny)+len(a.cfg.Safety.Ask)+len(a.cfg.Safety.Allow))
	for _, s := range a.cfg.Safety.Deny {
		rules = append(rules, permission.Rule{Behavior: permission.BEHAVIOR_DENY, Spec: s})
	}
	for _, s := range a.cfg.Safety.Ask {
		rules = append(rules, permission.Rule{Behavior: permission.BEHAVIOR_ASK, Spec: s})
	}
	for _, s := range a.cfg.Safety.Allow {
		rules = append(rules, permission.Rule{Behavior: permission.BEHAVIOR_ALLOW, Spec: s})
	}

	eng := &permission.Engine{Mode: permission.Mode(a.cfg.Safety.Mode), Rules: rules}
	if a.cfg.Safety.Fallback == spec.SAFETY_FALLBACK_AUTONOM {
		eng.Fallback = permission.DefaultApprovalPolicy{}
	}

	var sandbox tool.Sandbox
	if a.cfg.Safety.Sandbox {
		sandbox = tool.DefaultPolicy()
	}
	return eng, sandbox
}

// buildMiddleware resolves the configured ordered preset.
func (a *Agent) buildMiddleware(sandbox tool.Sandbox, perm core.ApprovalPolicy) middleware.Middleware {
	if a.cfg.Middleware == nil {
		return nil
	}
	switch a.cfg.Middleware.Preset {
	case spec.MIDDLEWARE_SECURE:
		// A typed nil interface would make preset.Secure install a gate
		// that always denies, so pass an untyped nil when there is none.
		if perm == nil {
			return preset.Secure(sandbox, nil)
		}
		return preset.Secure(sandbox, perm)
	case spec.MIDDLEWARE_DEFAULT:
		return preset.Default()
	default: // MIDDLEWARE_NONE
		return nil
	}
}

// --- stage 6 ---

// buildPersistence returns Host persistence only when file memory is enabled.
func (a *Agent) buildPersistence(ac *Host) (core.StateStore, core.WriteAheadLog) {
	if a.cfg.Memory == nil || a.cfg.Memory.Store != spec.MEMORY_STORE_FILE {
		return nil, nil
	}
	return ac.StateStore, ac.WAL
}

// buildSessions adds lineage management when sessions are enabled.
func (a *Agent) buildSessions(ac *Host, store core.StateStore, wal core.WriteAheadLog) (*session.Manager, error) {
	if a.cfg.Sessions == nil || store == nil {
		return nil, nil
	}
	dir := a.cfg.Sessions.Dir
	if dir == "" {
		dir = filepath.Join(ac.DataDir, "sessions")
	}
	m, err := session.NewManager(store, wal, dir)
	if err != nil {
		return nil, fmt.Errorf("agent: sessions: %w", err)
	}
	return m, nil
}

// --- stage 7 ---

// seedState builds the opening State: budget and autonomy from Limits,
// messages from the prompt builder.
func (a *Agent) seedState(ctx context.Context, ac *Host, b prompt.Builder, cwd string) (core.State, error) {
	autonomy, err := core.ParseAutonomyLevel(a.cfg.Limits.Autonomy)
	if err != nil {
		return core.State{}, fmt.Errorf("agent: limits.autonomy: %w", err)
	}
	state := core.State{
		RunID:          ac.RunID,
		ReasoningStyle: a.cfg.Reasoning.Style,
		Autonomy:       autonomy,
		Budget: core.Budget{
			MaxTurns:     a.cfg.Limits.MaxTurns,
			MaxRounds:    a.cfg.Limits.MaxRounds,
			MaxToolCalls: a.cfg.Limits.MaxToolCalls,
		},
	}
	msgs, err := b.Seed(ctx, prompt.Req{Cwd: cwd, State: state})
	if err != nil {
		return core.State{}, err
	}
	state.Messages = msgs
	return state, nil
}

// promptMaxBytes reads the configured cap, or 0 to let prompt apply its own.
func (a *Agent) promptMaxBytes() int {
	if a.cfg.Prompt == nil {
		return 0
	}
	return a.cfg.Prompt.MaxBytes
}

// startListener queues the first non-empty observation before returning,
// then leaves any remaining observations to the background pump.
func startListener(ctx context.Context, eng *Engine, src core.ObservationSource) error {
	observations := src.Observations(ctx)
	if observations == nil {
		return errNilObservationChan
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case obs, ok := <-observations:
			if !ok {
				if err := ctx.Err(); err != nil {
					return err
				}
				return errListenerClosed
			}
			if text := payloadToString(obs.Payload); text != "" {
				eng.Steer(text)
				go pumpListener(ctx, eng, observations)
				return nil
			}
		}
	}
}

// pumpListener forwards remaining non-empty observations to the
// concurrent-safe steering queue.
func pumpListener(ctx context.Context, eng *Engine, observations <-chan core.Observation) {
	for {
		select {
		case <-ctx.Done():
			return
		case obs, ok := <-observations:
			if !ok {
				return
			}
			if text := payloadToString(obs.Payload); text != "" {
				eng.Steer(text)
			}
		}
	}
}

// payloadToString flattens an observation into user text.
func payloadToString(p any) string {
	switch v := p.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
