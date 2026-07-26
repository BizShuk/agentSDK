package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/agent/permission"
	"github.com/bizshuk/agentsdk/agent/session"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/agent/wire"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware/preset"
	"github.com/bizshuk/agentsdk/planning"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/skill"
	builtin "github.com/bizshuk/agentsdk/tool"
)

// Agent is a prepared configuration plus its injected dependencies. It
// implements Runner, so a binary reduces to:
//
//	func main() { agent.Main(agent.MustNew(cfg)) }
//
// Construction is split in two on purpose. New validates and expands
// without touching the filesystem, so a bad config fails immediately and
// a test can build one without side effects. Bootstrap does the actual
// assembly, because it needs the AppConfig — data dir, run ID, state
// store, WAL — that agent.Run opens in its first step.
type Agent struct {
	cfg   Config
	deps  builder
	parts *Parts
}

// Parts exposes what Bootstrap assembled, for callers that drive the
// engine themselves instead of handing it to agent.Run: an interactive
// front end needs the session manager, a slash-command surface needs the
// skill registry.
//
// It is nil until Bootstrap has run.
type Parts struct {
	Engine    *runtime.Engine
	Sessions  *session.Manager
	Skills    *skill.Registry
	Prompt    prompt.Builder
	Config    Config
	AppConfig *AppConfig
	Cwd       string
}

// New prepares an agent. It expands the tier, validates the result, and
// records the injected dependencies — no I/O, no directories created, no
// provider contacted.
func New(cfg Config, opts ...Option) (*Agent, error) {
	// Checked before Prepare so the caller gets the actionable message.
	// spec would also reject a nameless config once persistence is on,
	// but it can only say "memory.store needs a name" — it does not know
	// that Once exists as the nameless alternative.
	if cfg.Name == "" {
		return nil, fmt.Errorf("agent: name is required (use Once for a nameless single call)")
	}
	prepared, err := cfg.Prepare()
	if err != nil {
		return nil, err
	}
	var b builder
	if err := b.apply(opts); err != nil {
		return nil, err
	}
	return &Agent{cfg: prepared, deps: b}, nil
}

// MustNew is New for entry points where a bad config is a programming
// error rather than a runtime condition.
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

// Parts returns what Bootstrap assembled, or nil before it has run.
func (a *Agent) Parts() *Parts { return a.parts }

// Preflight implements Preflighter: it builds the provider so a bad
// credential surfaces before the run leaves any trace on disk.
//
// A run that discovers a bad API key on its first model call has already
// created state and a WAL entry that will sit in `running` forever.
func (a *Agent) Preflight(_ context.Context, _ *AppConfig) error {
	if a.deps.provider != nil {
		return nil
	}
	_, err := provider.New(a.cfg.Model.Provider, provider.Options{
		Model:          a.cfg.Model.Name,
		BaseURL:        a.cfg.Model.BaseURL,
		APIKeyEnv:      a.cfg.Model.APIKeyEnv,
		CredentialKind: a.cfg.Model.CredentialKind,
	})
	return err
}

// Bootstrap implements Runner: it runs the assembly pipeline and
// returns the engine plus the opening state.
//
// The stage order is the knowledge this layer exists to hold. Each stage
// establishes what the next may assume, and three of the dependencies are
// not obvious from reading the stages alone:
//
//   - tools come after the provider, because the subagent task tool needs
//     a provider to run delegated work with
//   - the prompt comes after tools and skills, because the skill index is
//     part of the system message it seeds
//   - one permission.Engine instance feeds BOTH Engine.Approval and the
//     middleware chain; building two would let the gate and the chain
//     disagree about the same call
func (a *Agent) Bootstrap(ctx context.Context, ac *AppConfig) (*runtime.Engine, core.State, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, core.State{}, fmt.Errorf("agent: getwd: %w", err)
	}
	// ~/.config/<name>, the parent of the data dir — where user-level
	// skills, commands and agent definitions live.
	userDir := filepath.Dir(ac.DataDir)

	// --- stage 1: provider ---
	// An injected provider wins outright; otherwise the registry builds the
	// configured adapter. CredentialKind is passed through verbatim so the
	// strict modes ("oauth" / "api_key") error here with a clear message
	// rather than silently falling back to the legacy OAuth>API-key order.
	prov := a.deps.provider
	if prov == nil {
		prov, err = provider.New(a.cfg.Model.Provider, provider.Options{
			Model:          a.cfg.Model.Name,
			BaseURL:        a.cfg.Model.BaseURL,
			APIKeyEnv:      a.cfg.Model.APIKeyEnv,
			CredentialKind: a.cfg.Model.CredentialKind,
		})
		if err != nil {
			return nil, core.State{}, err
		}
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
	eng := runtime.NewEngine(step, prov, tools)
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

	// Spawn the listener pump last so the engine is fully wired before any
	// observation can Steer into it. The goroutine terminates when ctx is
	// cancelled (signal or deadline) OR when the listener closes its
	// channel — whichever happens first.
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

// buildTools assembles the tool registry: the built-in allowlist, then
// injected tools, then the subagent task tool.
//
// A nil registry is returned when nothing is configured. That is not an
// oversight — runtime treats a nil ToolRegistry as "no tools", the model
// then receives no tool specs, and the loop reduces to plain conversation.
func (a *Agent) buildTools(cwd, userDir string, prov core.Provider) (core.ToolRegistry, error) {
	if a.cfg.Tools == nil && a.cfg.Subagents == nil && len(a.deps.tools) == 0 {
		return nil, nil
	}
	reg := action.NewRegistry()

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

	if a.cfg.Subagents != nil {
		defs := discoverSubagentDefs(a.cfg, userDir, cwd)
		if len(defs) > 0 {
			reg.Register(skill.NewSpawner(a.subagentRunner(prov), defs...))
		}
	}
	return reg, nil
}

// registerBuiltins registers the requested built-in tools. An empty
// allowlist means all six — the config says which tools to have, and
// saying nothing means the standard set.
func registerBuiltins(reg *action.Registry, allow []string, workDir string) error {
	policy := action.DefaultPolicy()
	if len(allow) == 0 {
		if _, err := builtin.RegisterDefaults(reg, builtin.Options{Policy: policy, WorkingDir: workDir}); err != nil {
			return fmt.Errorf("agent: register built-in tools: %w", err)
		}
		return nil
	}
	for _, name := range allow {
		switch name {
		case spec.TOOL_READ:
			reg.Register(builtin.NewRead(builtin.ReadOptions{}, policy, workDir))
		case spec.TOOL_GLOB:
			reg.Register(builtin.NewGlob(builtin.GlobOptions{}, policy, workDir))
		case spec.TOOL_GREP:
			reg.Register(builtin.NewGrep(builtin.GrepOptions{}, policy, workDir))
		case spec.TOOL_WRITE:
			t, err := builtin.NewWrite(builtin.WriteOptions{}, policy, workDir)
			if err != nil {
				return fmt.Errorf("agent: write tool: %w", err)
			}
			reg.Register(t)
		case spec.TOOL_EDIT:
			t, err := builtin.NewEdit(builtin.EditOptions{}, policy, workDir)
			if err != nil {
				return fmt.Errorf("agent: edit tool: %w", err)
			}
			reg.Register(t)
		case spec.TOOL_BASH:
			t, err := builtin.NewBash(builtin.BashOptions{}, policy, workDir)
			if err != nil {
				return fmt.Errorf("agent: bash tool: %w", err)
			}
			reg.Register(t)
		default:
			// spec.Validate rejects unknown names; reaching here means
			// the two lists drifted.
			return fmt.Errorf("agent: unknown built-in tool %q", name)
		}
	}
	return nil
}

// discoverSubagentDefs merges user-level and project-level definitions.
// Project comes second so it wins a name clash.
func discoverSubagentDefs(cfg Config, userDir, cwd string) []skill.Def {
	dirs := cfg.Subagents.Dirs
	if len(dirs) == 0 {
		dirs = discoveryRoots(cfg, userDir, cwd, "agents")
	}
	var defs []skill.Def
	for _, dir := range dirs {
		found, err := skill.DiscoverDefs(dir)
		if err != nil {
			continue // a missing definitions directory is normal
		}
		defs = append(defs, found...)
	}
	return defs
}

// subagentRunner gives each delegation a scoped, ephemeral engine over the
// same provider: no store, no WAL, its own turn budget. A subagent's work
// is part of the parent's turn, not a run of its own.
func (a *Agent) subagentRunner(prov core.Provider) skill.RunFunc {
	maxTurns := a.cfg.Subagents.MaxTurns
	return func(ctx context.Context, def skill.Def, promptText string) (string, error) {
		reg := action.NewRegistry()
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
		step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
			core.REASON_REACT: planning.NewThinkThenAct(),
		})
		sub := runtime.NewEngine(step, prov, reg)
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

// buildDecide registers every enabled rule and returns the dispatcher.
//
// Enable decides what is registered; State.ReasoningStyle decides which
// one runs. Registering only the selected style is the common case, and
// spec.Validate has already guaranteed the selected style is in the list.
func (a *Agent) buildDecide() (core.Decide, error) {
	rules := map[core.ReasoningStyle]core.DecisionRule{}
	for _, name := range a.cfg.Reasoning.Enable {
		rule, err := ruleFor(core.ReasoningStyle(name))
		if err != nil {
			return nil, err
		}
		rules[rule.Kind()] = rule
	}
	// Injected rules win, so an application can replace a built-in
	// strategy with its own implementation of the same Kind.
	for _, rule := range a.deps.rules {
		rules[rule.Kind()] = rule
	}
	return core.NewDecide(rules), nil
}

// ruleFor maps a style name to its implementation. This is the one place
// that knows planning exists, which is why spec can enumerate styles from
// core's constants without importing planning.
func ruleFor(style core.ReasoningStyle) (core.DecisionRule, error) {
	switch style {
	case core.REASON_REACT:
		return planning.NewThinkThenAct(), nil
	case core.REASON_PLAN_THEN_RUN:
		return planning.NewPlanThenRun(), nil
	case core.REASON_DO_THEN_REVIEW:
		return planning.NewRunThenReview(), nil
	case core.REASON_ONE_SHOT:
		return planning.NewOneShotReasoning(), nil
	case core.REASON_LEARN_FROM_FAILURE:
		return planning.NewLearnFromFailure(), nil
	case core.REASON_PICK_AGENT:
		return planning.NewChooseAgent(), nil
	default:
		return nil, fmt.Errorf("agent: no rule for reasoning style %q", style)
	}
}

// --- stage 5 ---

// buildSafety returns the approval engine and the sandbox. Both may be
// nil, which the engine and the middleware chain treat as "no gate".
//
// The SAME *permission.Engine goes to Engine.Approval and into the
// middleware chain. Two instances would be two policies that happen to
// look alike today and diverge tomorrow.
func (a *Agent) buildSafety() (*permission.Engine, action.Sandbox) {
	if a.cfg.Safety == nil {
		return nil, nil
	}
	rules := make([]permission.Rule, 0,
		len(a.cfg.Safety.Deny)+len(a.cfg.Safety.Ask)+len(a.cfg.Safety.Allow))
	// Deny first, then ask, then allow. The engine applies deny > ask >
	// allow regardless of order, but keeping the slice in precedence
	// order makes a dump of the rules readable.
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
		eng.Fallback = action.DefaultApprovalPolicy{}
	}

	var sandbox action.Sandbox
	if a.cfg.Safety.Sandbox {
		sandbox = action.DefaultPolicy()
	}
	return eng, sandbox
}

// buildMiddleware resolves the preset. The chain's ORDER is a correctness
// property, not a preference, so config picks a preset rather than
// composing layers.
func (a *Agent) buildMiddleware(sandbox action.Sandbox, perm core.ApprovalPolicy) middlewareChain {
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

// buildPersistence returns the state store and WAL, or nils when the
// memory block is off. agent.Run backfills from AppConfig only when the
// engine leaves them nil, so returning nils here genuinely disables
// persistence rather than deferring to the default.
func (a *Agent) buildPersistence(ac *AppConfig) (core.StateStore, core.WriteAheadLog) {
	if a.cfg.Memory == nil || a.cfg.Memory.Store != spec.MEMORY_STORE_FILE {
		return nil, nil
	}
	return ac.StateStore, ac.WAL
}

// buildSessions adds lineage — list, resume, fork, tree — on top of the
// store. Without persistence there is nothing to have lineage over, which
// spec.Validate already rejects.
func (a *Agent) buildSessions(ac *AppConfig, store core.StateStore, wal core.WriteAheadLog) (*session.Manager, error) {
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
func (a *Agent) seedState(ctx context.Context, ac *AppConfig, b prompt.Builder, cwd string) (core.State, error) {
	state := core.State{
		RunID:          ac.RunID,
		ReasoningStyle: core.ReasoningStyle(a.cfg.Reasoning.Style),
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

// pumpListener drains an ObservationSource and forwards each payload to
// Engine.Steer so the next Decide cycle sees it as a user message. The
// goroutine exits when ctx is cancelled OR when the source closes its
// channel — whichever happens first.
//
// Engine.Steer is concurrent-safe, so the goroutine can run alongside
// the engine loop without further synchronization. Empty payloads are
// dropped here rather than queued, since the engine's no-message-loop
// would otherwise block on them.
func pumpListener(ctx context.Context, eng *runtime.Engine, src core.ObservationSource) {
	for obs := range src.Observations(ctx) {
		if ctx.Err() != nil {
			return
		}
		if text := payloadToString(obs.Payload); text != "" {
			eng.Steer(text)
		}
	}
}

// payloadToString flattens an observation payload into a single user
// message. The most common case is a string from a log listener; the
// Stringer fallback covers structured payloads that know how to render
// themselves; the catch-all handles anything else with %v formatting.
// Empty results drop the observation (Engine.Steer would also filter
// empties, but skipping here keeps the queue clean).
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
