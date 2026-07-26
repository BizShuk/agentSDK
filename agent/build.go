package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bizshuk/agentsdk/agent/permission"
	"github.com/bizshuk/agentsdk/agent/session"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/agent/wire"
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

	// --- stage 7: output ---
	sink := a.buildSink()

	// --- stage 8: assemble ---
	eng := NewEngine(step, prov, tools)
	eng.Middleware = a.buildMiddleware(sandbox, perm)
	eng.Store = store
	eng.Log = wal
	eng.Sink = sink
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

	// Start listeners only after the engine is fully wired.
	if a.deps.listener != nil {
		go pumpListener(ctx, eng, a.deps.listener)
	}

	a.parts = &Parts{
		Engine: eng, Sessions: sessions, Skills: skills, Prompt: builderP,
		Config: a.cfg, AppConfig: ac, Cwd: cwd,
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
		if err := registerBuiltins(reg, a.cfg.Tools.Builtin, workDir); err != nil {
			return nil, err
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
			spawner := skill.NewSpawner(a.subagentRunner(prov), defs...)
			spawner.MaxDepth = a.cfg.Subagents.MaxDepth
			reg.Register(spawner)
		}
	}
	return reg, nil
}

// registerBuiltins treats an empty allowlist as all built-ins.
func registerBuiltins(reg *tool.Registry, allow []string, workDir string) error {
	policy := tool.DefaultPolicy()
	if len(allow) == 0 {
		if err := builtin.RegisterDefaults(reg, builtin.Options{Policy: policy, WorkingDir: workDir}); err != nil {
			return fmt.Errorf("agent: register built-in tools: %w", err)
		}
		return nil
	}
	for _, name := range allow {
		switch name {
		case builtin.NAME_READ:
			r := builtin.NewRead(policy, workDir)
			reg.Register(r)
		case builtin.NAME_GLOB:
			g := builtin.NewGlob(policy, workDir)
			reg.Register(g)
		case builtin.NAME_GREP:
			gr := builtin.NewGrep(policy, workDir)
			reg.Register(gr)
		case builtin.NAME_WRITE:
			w, err := builtin.NewWrite(policy, workDir)
			if err != nil {
				return fmt.Errorf("agent: write tool: %w", err)
			}
			reg.Register(w)
		case builtin.NAME_EDIT:
			e, err := builtin.NewEdit(policy, workDir)
			if err != nil {
				return fmt.Errorf("agent: edit tool: %w", err)
			}
			reg.Register(e)
		case builtin.NAME_BASH:
			b, err := builtin.NewBash(policy, workDir)
			if err != nil {
				return fmt.Errorf("agent: bash tool: %w", err)
			}
			reg.Register(b)
		default:
			return fmt.Errorf("agent: unknown built-in tool %q", name)
		}
	}
	return nil
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
func (a *Agent) subagentRunner(prov core.Provider) skill.RunFunc {
	maxTurns := a.cfg.Subagents.MaxTurns
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
			if err := registerBuiltins(reg, allow, workDir); err != nil {
				return "", err
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
			Autonomy:       autonomyLevel(a.cfg.Limits.Autonomy),
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
	}
}

// --- stage 3 ---

// buildDecide registers enabled rules; injected rules override by Kind.
func (a *Agent) buildDecide() (core.Decide, error) {
	rules := map[string]reasoning.DecisionRule{}
	for _, name := range a.cfg.Reasoning.Enable {
		rule, err := ruleFor(name)
		if err != nil {
			return nil, err
		}
		rules[rule.Kind()] = rule
	}
	for _, rule := range a.deps.rules {
		rules[rule.Kind()] = rule
	}
	return reasoning.NewDecide(rules), nil
}

// ruleFor maps declarative reasoning styles to implementations.
func ruleFor(style string) (reasoning.DecisionRule, error) {
	switch style {
	case core.REASON_REACT:
		return reasoning.NewThinkThenAct(), nil
	case core.REASON_PLAN_THEN_RUN:
		return reasoning.NewPlanThenRun(), nil
	case core.REASON_DO_THEN_REVIEW:
		return reasoning.NewRunThenReview(), nil
	case core.REASON_ONE_SHOT:
		return reasoning.NewOneShotReasoning(), nil
	case core.REASON_LEARN_FROM_FAILURE:
		return reasoning.NewLearnFromFailure(), nil
	case core.REASON_PICK_AGENT:
		return reasoning.NewChooseAgent(), nil
	default:
		return nil, fmt.Errorf("agent: no rule for reasoning style %q", style)
	}
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

// buildSink resolves the presentation stream. An injected sink wins over
// the configured format.
func (a *Agent) buildSink() core.EventSink {
	if a.deps.sink != nil {
		return a.deps.sink
	}
	if a.cfg.Output == nil {
		return nil
	}
	switch a.cfg.Output.Format {
	case spec.OUTPUT_JSON:
		return wire.NewSink(os.Stdout)
	default:
		// text and tui are rendered by the front end, which owns the
		// terminal; the engine emits nothing on its own.
		return nil
	}
}

// --- stage 8 ---

// seedState builds the opening State: budget and autonomy from Limits,
// messages from the prompt builder.
func (a *Agent) seedState(ctx context.Context, ac *Host, b prompt.Builder, cwd string) (core.State, error) {
	state := core.State{
		RunID:          ac.RunID,
		ReasoningStyle: a.cfg.Reasoning.Style,
		Autonomy:       autonomyLevel(a.cfg.Limits.Autonomy),
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

// autonomyLevel maps the config string to the core constant. spec has
// already validated the value, so an unrecognized one can only mean the
// two lists drifted; L2 is the safe reading (low-risk automatic,
// high-risk asks).
func autonomyLevel(s string) core.AutonomyLevel {
	switch s {
	case "L0":
		return core.AUTONOMY_L0
	case "L1":
		return core.AUTONOMY_L1
	case "L3":
		return core.AUTONOMY_L3
	case "L4":
		return core.AUTONOMY_L4
	default:
		return core.AUTONOMY_L2
	}
}

// pumpListener forwards non-empty observations to the concurrent-safe
// steering queue.
func pumpListener(ctx context.Context, eng *Engine, src core.ObservationSource) {
	for obs := range src.Observations(ctx) {
		if ctx.Err() != nil {
			return
		}
		if text := payloadToString(obs.Payload); text != "" {
			eng.Steer(text)
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
