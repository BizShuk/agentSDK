// Package spec is the declarative half of the agent skeleton: the Config
// schema an application writes (by hand or from a YAML/JSON file), the
// Choice metadata a wizard enumerates, tier expansion, and validation.
//
// It imports core and the standard library only. Nothing here builds a
// runtime object — that is agent's job. The split exists so that anything
// which merely needs to READ or PRODUCE a config (wizard, schema
// generator, validation tool, web form) does not drag in gosdk, auth,
// middleware, or a provider SDK.
//
// Two-layer opt-in, one rule:
//
//	layer 1 feature — a block is a pointer: absent key = nil = off,
//	                  empty object = on with defaults
//	layer 2 variant — a named field inside the block picks the
//	                  implementation: empty string = that feature's default
//
// See plans/2026-07-22-agent-skeleton-config-opt-in.md.
package spec

// Config is the whole declarative surface. Every field is serializable;
// anything that cannot be spelled in JSON (a live Provider, a closure
// hook, a custom tool) is injected through agent.Option instead.
type Config struct {
	// Name is the application identifier — it resolves ~/.config/<Name>
	// and keys persisted state, so it must be stable across runs.
	// Required from TIER_BASIC up; TIER_ONESHOT has no persistence and
	// therefore no need for it.
	Name string `json:"name,omitempty"`

	// Tier is the engagement ladder: how much of the SDK is wired.
	// Empty means TIER_BASIC. Expansion is monotonic — each tier is a
	// superset of the one below — and explicit blocks always win over
	// whatever the tier turned on.
	Tier string `json:"tier,omitempty"`

	// Persona is the fixed identity text, available at EVERY tier
	// (TIER_ONESHOT included — it is the equivalent of the provider
	// CLI's --system flag). Content the agent collects from elsewhere
	// (context files, skill index, env) belongs in Prompt instead; the
	// dividing line is where the text comes from, not what it says.
	Persona string `json:"persona,omitempty"`

	Model     Model     `json:"model"`
	Reasoning Reasoning `json:"reasoning,omitzero"`
	Limits    Limits    `json:"limits,omitzero"`

	// Feature blocks — layer 1 of the opt-in. nil is off.
	Middleware *Middleware `json:"middleware,omitempty"`
	Memory     *Memory     `json:"memory,omitempty"`
	Tools      *Tools      `json:"tools,omitempty"`
	Safety     *Safety     `json:"safety,omitempty"`
	Prompt     *Prompt     `json:"prompt,omitempty"`
	Skills     *Skills     `json:"skills,omitempty"`
	Subagents  *Subagents  `json:"subagents,omitempty"`
	Sessions   *Sessions   `json:"sessions,omitempty"`
	Output     *Output     `json:"output,omitempty"`
	Telemetry  *Telemetry  `json:"telemetry,omitempty"`
}

// Model selects the provider adapter. Provider is the registry key; the
// rest are optional overrides — every adapter already reads its own
// credential and base-URL environment, so an empty Config still works
// when the environment is set up.
type Model struct {
	Provider  string `json:"provider,omitempty"`    // registry key; empty = PROVIDER_DEFAULT
	Name      string `json:"name,omitempty"`        // empty = the adapter's own flagship default
	BaseURL   string `json:"base_url,omitempty"`    // empty = the adapter's own default endpoint
	APIKeyEnv string `json:"api_key_env,omitempty"` // empty = the adapter's own convention
}

// Reasoning carries the two independent planning decisions.
//
// Style is WHICH RULE RUNS: it seeds core.State.ReasoningStyle, and the
// engine dispatches on that field every step.
//
// Enable is WHICH RULES ARE REGISTERED in the core.NewDecide map. Empty
// means "just Style", which is what almost every agent wants. Register
// more only when the run legitimately switches mid-flight — choose_agent
// acting as a router, or learn_from_failure taking over a failed turn.
// Dispatching to an unregistered style makes NewDecide emit a NOTIFY
// error rather than reason, so Validate rejects Style ∉ Enable early.
type Reasoning struct {
	Style  string   `json:"style,omitempty"`
	Enable []string `json:"enable,omitempty"`
}

// Limits bounds one run. Zero values mean "no bound from config" — the
// tier defaults fill in a sane MaxTurns, and agent.WithTimeout still caps
// wall-clock time at the process level.
// MaxTurns and MaxRounds measure different things and both are kept:
// a round is one model request plus the tool calls it triggers (what an
// operator counts), a turn is one Decide iteration (what loopguard
// counts). MaxToolCalls bounds a single round's tool-call batch.
type Limits struct {
	MaxTurns     int    `json:"max_turns,omitempty"`
	MaxRounds    int    `json:"max_rounds,omitempty"`
	MaxToolCalls int    `json:"max_tool_calls,omitempty"`
	MaxWallTime  string `json:"max_wall_time,omitempty"` // Go duration string, e.g. "10m"
	Autonomy     string `json:"autonomy,omitempty"`      // L0..L4; empty = L2
}

// Middleware picks a preset chain rather than exposing chain assembly to
// config — ordering is a correctness property (retry outermost, sanitizer
// innermost), not a knob.
type Middleware struct {
	Preset string `json:"preset,omitempty"` // none | default | secure
}

// Memory turns on persistence. Store="file" gives resume and crash
// recovery; it also makes Config.Name required, since the state and WAL
// live under ~/.config/<Name>.
type Memory struct {
	Store      string `json:"store,omitempty"`      // none | file
	Compaction string `json:"compaction,omitempty"` // none | headline
}

// Tools enables the built-in tool set. Builtin is an allowlist: empty
// means all six. WorkingDir empty means the process working directory.
type Tools struct {
	Builtin    []string `json:"builtin,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`
}

// Safety is the two orthogonal gates: Mode × rules decide WHO approves,
// Sandbox decides WHAT a call may touch.
//
// Rule specs use the permission package's syntax — "bash(sudo:*)",
// "edit(src/**)". Precedence is deny > ask > allow.
type Safety struct {
	Mode     string   `json:"mode,omitempty"` // default | acceptEdits | plan | bypassPermissions
	Deny     []string `json:"deny,omitempty"`
	Ask      []string `json:"ask,omitempty"`
	Allow    []string `json:"allow,omitempty"`
	Sandbox  bool     `json:"sandbox,omitempty"`
	Fallback string   `json:"fallback,omitempty"` // none | autonomy
}

// Prompt configures content COLLECTED from the environment. The fixed
// identity string lives in Config.Persona instead, because that one must
// work at TIER_ONESHOT where this block does not exist.
type Prompt struct {
	Sources    []string `json:"sources,omitempty"`     // files | skills | env | reminder
	UserDir    string   `json:"user_dir,omitempty"`    // empty = ~/.config/<Name>
	ProjectDir string   `json:"project_dir,omitempty"` // empty = DEFAULT_PROJECT_DIR
	MaxBytes   int      `json:"max_bytes,omitempty"`   // empty = the loader's own cap
}

// Skills enables SKILL.md / command discovery. Empty Dirs means the
// conventional pair: <user_dir>/skills and <cwd>/<project_dir>/skills.
type Skills struct {
	Dirs []string `json:"dirs,omitempty"`
}

// Subagents enables the task tool. MaxDepth guards against a subagent
// spawning subagents without bound.
type Subagents struct {
	Dirs     []string `json:"dirs,omitempty"`
	MaxDepth int      `json:"max_depth,omitempty"` // 0 = DEFAULT_SUBAGENT_DEPTH
	MaxTurns int      `json:"max_turns,omitempty"` // 0 = min(Limits.MaxTurns, DEFAULT_SUBAGENT_TURNS)
}

// Sessions adds lineage on top of the state store: list, resume, fork,
// tree. The transcript itself is still the WAL.
type Sessions struct {
	Dir string `json:"dir,omitempty"` // empty = <data_dir>/sessions
}

// Output selects the presentation sink bound to core.EventSink.
type Output struct {
	Format string `json:"format,omitempty"` // text | json | tui
}

// Telemetry turns on OpenTelemetry tracing.
type Telemetry struct {
	Enabled bool   `json:"enabled,omitempty"`
	Service string `json:"service,omitempty"` // empty = Config.Name
}

// Clone returns a deep copy. Tier expansion and validation both mutate,
// and a caller's literal must never be modified underneath it.
func (c Config) Clone() Config {
	out := c
	out.Reasoning.Enable = cloneStrings(c.Reasoning.Enable)
	out.Middleware = clonePtr(c.Middleware)
	out.Memory = clonePtr(c.Memory)
	out.Sessions = clonePtr(c.Sessions)
	out.Output = clonePtr(c.Output)
	out.Telemetry = clonePtr(c.Telemetry)
	if c.Tools != nil {
		t := *c.Tools
		t.Builtin = cloneStrings(c.Tools.Builtin)
		out.Tools = &t
	}
	if c.Safety != nil {
		s := *c.Safety
		s.Deny, s.Ask, s.Allow = cloneStrings(c.Safety.Deny), cloneStrings(c.Safety.Ask), cloneStrings(c.Safety.Allow)
		out.Safety = &s
	}
	if c.Prompt != nil {
		p := *c.Prompt
		p.Sources = cloneStrings(c.Prompt.Sources)
		out.Prompt = &p
	}
	if c.Skills != nil {
		s := *c.Skills
		s.Dirs = cloneStrings(c.Skills.Dirs)
		out.Skills = &s
	}
	if c.Subagents != nil {
		s := *c.Subagents
		s.Dirs = cloneStrings(c.Subagents.Dirs)
		out.Subagents = &s
	}
	return out
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
