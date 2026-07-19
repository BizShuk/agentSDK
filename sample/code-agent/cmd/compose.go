// compose.go is the composition root proper: every harness capability is
// constructed here and injected into runtime.Engine — the DI seam the
// harness modularization plan prescribes. Nothing below runtime knows any
// of these packages exist.
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/action"
	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/contextfile"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/hook"
	"github.com/bizshuk/agentsdk/permission"
	"github.com/bizshuk/agentsdk/planning"
	anthropicprovider "github.com/bizshuk/agentsdk/provider/anthropic"
	minimaxprovider "github.com/bizshuk/agentsdk/provider/minimax"
	"github.com/bizshuk/agentsdk/runtime"
	"github.com/bizshuk/agentsdk/session"
	"github.com/bizshuk/agentsdk/skill"
	"github.com/bizshuk/agentsdk/subagent"
	builtin "github.com/bizshuk/agentsdk/tool"
)

// PROJECT_DIR is the project-local harness directory (skills / commands /
// agents), the `.claude` / `.pi` analog.
const PROJECT_DIR = ".agentsdk"

// agentParts is everything the mode surfaces (interactive / headless) need.
type agentParts struct {
	engine   *runtime.Engine
	sessions *session.Manager
	skills   *skill.Registry
	cfg      *config.AppConfig
	cwd      string
	system   string // merged system prompt (context files + skill listing)
	maxTurns int
}

type composeOptions struct {
	cfg            *config.AppConfig
	fake           bool
	provider       string // "minimax" | "anthropic"; ignored when fake
	model          string // "" → adapter's own flagship default
	apiKey         string
	baseURL        string
	maxTurns       int
	permissionMode string
}

// compose builds provider → tools → harness pieces → engine, in DI order.
func compose(o composeOptions) (*agentParts, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	userRoot := filepath.Dir(o.cfg.DataDir) // ~/.config/code-agent

	// --- provider ---
	prov, err := buildProvider(o)
	if err != nil {
		return nil, err
	}

	// --- tools: 6 built-ins + subagent task tool ---
	reg := action.NewRegistry()
	if _, err := builtin.RegisterDefaults(reg, builtin.Options{
		Policy:     action.DefaultPolicy(),
		WorkingDir: cwd,
	}); err != nil {
		return nil, fmt.Errorf("register built-in tools: %w", err)
	}
	defs := discoverSubagents(userRoot, cwd)
	if len(defs) > 0 {
		reg.Register(subagent.NewSpawner(subagentRunner(prov, o.maxTurns), defs...))
	}

	// --- contextfile: user → repo root → cwd instruction hierarchy ---
	sysText, _, err := contextfile.Loader{UserDir: userRoot}.Load(cwd)
	if err != nil {
		return nil, fmt.Errorf("context files: %w", err)
	}

	// --- skills / commands (progressive disclosure) ---
	skills := skill.NewRegistry()
	for _, dir := range []string{filepath.Join(userRoot, "skills"), filepath.Join(cwd, PROJECT_DIR, "skills")} {
		if err := skills.DiscoverSkills(dir); err != nil {
			return nil, err
		}
	}
	for _, dir := range []string{filepath.Join(userRoot, "commands"), filepath.Join(cwd, PROJECT_DIR, "commands")} {
		if err := skills.DiscoverCommands(dir); err != nil {
			return nil, err
		}
	}
	system := mergeSystem(sysText, skills.SystemPrompt())

	// --- permission: mode × rules, autonomy grid as fallback ---
	perm := &permission.Engine{
		Mode: permission.Mode(o.permissionMode),
		Rules: []permission.Rule{
			{Behavior: permission.BEHAVIOR_DENY, Spec: "bash(sudo:*)"},
			{Behavior: permission.BEHAVIOR_ASK, Spec: "bash(git push:*)"},
		},
		Fallback: action.DefaultApprovalPolicy{},
	}

	// --- hooks: one safety gate as the demo rule ---
	hooks := hook.NewRunner(hook.Rule{
		Event: core.HOOK_PRE_TOOL_USE,
		Match: "bash",
		Handlers: []hook.Handler{hook.Func(func(_ context.Context, ev core.HookEvent) (core.HookDecision, error) {
			if cmdStr, _ := ev.ToolCall.Args["command"].(string); strings.Contains(cmdStr, "rm -rf /") {
				return core.HookDecision{Block: true, Reason: "refusing rm -rf on root-ish path"}, nil
			}
			return core.HookDecision{}, nil
		})},
	})

	// --- engine assembly (ports; nil stays no-op) ---
	step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
		core.REASON_REACT: planning.NewThinkThenAct(),
	})
	eng := runtime.NewEngine(step, prov, reg)
	eng.Middleware = config.SecureMiddleware(action.DefaultPolicy(), perm)
	eng.Approval = perm
	eng.Hooks = hooks
	eng.Store = o.cfg.StateStore
	eng.Log = o.cfg.WAL

	sessions, err := session.NewManager(o.cfg.StateStore, o.cfg.WAL, filepath.Join(o.cfg.DataDir, "sessions"))
	if err != nil {
		return nil, err
	}

	return &agentParts{
		engine: eng, sessions: sessions, skills: skills, cfg: o.cfg,
		cwd: cwd, system: system, maxTurns: o.maxTurns,
	}, nil
}

// buildProvider selects the core.Provider from options. Each real adapter
// reads its own credentials (MINIMAX_API_KEY / ANTHROPIC_API_KEY) and its
// own base-URL env, so --api-key / --base-url are optional overrides. An
// empty --model lets the adapter use its own flagship default.
func buildProvider(o composeOptions) (core.Provider, error) {
	if o.fake {
		return newFakeProvider(), nil
	}
	switch strings.ToLower(o.provider) {
	case "", "minimax":
		opts := []minimaxprovider.Option{}
		if o.model != "" {
			opts = append(opts, minimaxprovider.WithModel(o.model))
		}
		if o.apiKey != "" {
			opts = append(opts, minimaxprovider.WithAPIKey(o.apiKey))
		}
		if o.baseURL != "" {
			opts = append(opts, minimaxprovider.WithBaseURL(o.baseURL))
		}
		p, err := minimaxprovider.New(opts...) // reads MINIMAX_API_KEY / MINIMAX_BASE_URL
		if err != nil {
			return nil, fmt.Errorf("minimax provider: %w", err)
		}
		return p, nil
	case "anthropic":
		opts := []anthropicprovider.Option{}
		if o.model != "" {
			opts = append(opts, anthropicprovider.WithModel(o.model))
		}
		if o.apiKey != "" {
			opts = append(opts, anthropicprovider.WithAPIKey(o.apiKey))
		}
		if o.baseURL != "" {
			opts = append(opts, anthropicprovider.WithBaseURL(o.baseURL))
		}
		p, err := anthropicprovider.New(opts...) // reads ANTHROPIC_API_KEY
		if err != nil {
			return nil, fmt.Errorf("anthropic provider: %w", err)
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (want: minimax | anthropic; or --fake)", o.provider)
	}
}

// sessionRequest selects which conversation openState starts from.
type sessionRequest struct {
	continueLatest bool
	resumeID       string
	forkID         string
}

// openState returns the State to run: a fresh session, the latest one,
// a specific resume, or a fork copy.
func (p *agentParts) openState(ctx context.Context, req sessionRequest) (core.State, error) {
	switch {
	case req.forkID != "":
		meta, err := p.sessions.Fork(ctx, req.forkID, "fork of "+req.forkID)
		if err != nil {
			return core.State{}, err
		}
		return p.cfg.StateStore.Load(ctx, meta.ID)
	case req.resumeID != "":
		return p.cfg.StateStore.Load(ctx, req.resumeID)
	case req.continueLatest:
		meta, err := p.sessions.Latest(p.cwd)
		if err != nil {
			return core.State{}, err
		}
		return p.cfg.StateStore.Load(ctx, meta.ID)
	default:
		if _, err := p.sessions.Begin(p.cfg.RunID, "", p.cwd); err != nil {
			return core.State{}, err
		}
		return p.newState(), nil
	}
}

func (p *agentParts) newState() core.State {
	st := core.State{
		RunID:          p.cfg.RunID,
		ReasoningStyle: core.REASON_REACT,
		Autonomy:       core.AUTONOMY_L2,
		Budget:         core.Budget{MaxTurns: p.maxTurns},
	}
	if p.system != "" {
		st.Messages = append(st.Messages, core.Message{
			Role:  core.ROLE_SYSTEM,
			Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: p.system}},
			Ts:    time.Now().UTC(),
		})
	}
	return st
}

// discoverSubagents merges user-level and project-level definitions
// (project wins on name clash via later registration order in the map).
func discoverSubagents(userRoot, cwd string) []subagent.Def {
	var defs []subagent.Def
	for _, dir := range []string{filepath.Join(userRoot, "agents"), filepath.Join(cwd, PROJECT_DIR, "agents")} {
		found, err := subagent.DiscoverDefs(dir)
		if err != nil {
			continue
		}
		defs = append(defs, found...)
	}
	return defs
}

// subagentRunner is the injected RunFunc: each delegation gets a scoped,
// ephemeral engine (no store/WAL) over the same provider.
func subagentRunner(prov core.Provider, maxTurns int) subagent.RunFunc {
	return func(ctx context.Context, def subagent.Def, prompt string) (string, error) {
		reg := action.NewRegistry()
		if _, err := builtin.RegisterDefaults(reg, builtin.Options{
			Policy:     action.DefaultPolicy(),
			WorkingDir: ".",
		}); err != nil {
			return "", err
		}
		step := core.NewDecide(map[core.ReasoningStyle]core.DecisionRule{
			core.REASON_REACT: planning.NewThinkThenAct(),
		})
		sub := runtime.NewEngine(step, prov, reg)
		sub.Middleware = config.SecureMiddleware(action.DefaultPolicy(), action.DefaultApprovalPolicy{})
		sub.Approval = action.DefaultApprovalPolicy{}

		st := core.State{
			RunID:          fmt.Sprintf("sub-%d", time.Now().UnixNano()),
			ReasoningStyle: core.REASON_REACT,
			Autonomy:       core.AUTONOMY_L2,
			Budget:         core.Budget{MaxTurns: min(maxTurns, 10)},
		}
		if def.Prompt != "" {
			st.Messages = append(st.Messages, core.Message{
				Role:  core.ROLE_SYSTEM,
				Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: def.Prompt}},
				Ts:    time.Now().UTC(),
			})
		}
		st.Messages = append(st.Messages, userMessage(prompt))
		final, err := sub.Run(ctx, st)
		if err != nil {
			return "", err
		}
		return lastAssistantText(final), nil
	}
}

func mergeSystem(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, strings.TrimSpace(p))
		}
	}
	return strings.Join(kept, "\n\n")
}

func userMessage(text string) core.Message {
	return core.Message{
		Role:  core.ROLE_USER,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}},
		Ts:    time.Now().UTC(),
	}
}

// lastAssistantText returns the newest assistant text in the transcript.
func lastAssistantText(s core.State) string {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role != core.ROLE_ASSISTANT {
			continue
		}
		var sb strings.Builder
		for _, p := range s.Messages[i].Parts {
			if p.Kind == core.PART_KIND_PLAIN_TEXT && p.Text != "" {
				sb.WriteString(p.Text)
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}
	return ""
}
