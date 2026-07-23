package spec

import (
	"fmt"
	"slices"
)

// Tier names. The ladder is monotonic: each tier turns on everything the
// tier below it did, plus its own additions.
const (
	TIER_ONESHOT  = "oneshot"  // provider only — one model call, no persistence
	TIER_BASIC    = "basic"    // + reasoning loop, middleware, state/WAL
	TIER_STANDARD = "standard" // + built-in tools, permission, sessions, context files
	TIER_FULL     = "full"     // + skills, subagents, hooks, stream output
)

// Defaults referenced by tier expansion. They are constants rather than
// magic numbers inside Expand so a reader can see the whole default
// posture in one place.
const (
	DEFAULT_TIER            = TIER_BASIC
	DEFAULT_STYLE           = "think_then_act"
	DEFAULT_PROVIDER        = "minimax"
	DEFAULT_PROJECT_DIR     = ".agentsdk"
	DEFAULT_AUTONOMY        = "L2"
	DEFAULT_MAX_TURNS       = 20
	DEFAULT_SUBAGENT_DEPTH  = 1
	DEFAULT_SUBAGENT_TURNS  = 10
	DEFAULT_ONESHOT_TURNS   = 2
	DEFAULT_STANDARD_TURNS  = 40

	// Rounds are the operator-facing budget; turns are the internal
	// guard. The ladder mirrors the *_TURNS one branch for branch, and
	// stays below it — a round costs at least one turn, and for ReAct
	// about three.
	DEFAULT_MAX_ROUNDS      = 10
	DEFAULT_ONESHOT_ROUNDS  = 1
	DEFAULT_STANDARD_ROUNDS = 30

	// Tool-call ceilings per round. TIER_ONESHOT registers no tools, so
	// its bound is left unset (0 = unbounded) rather than given a value
	// that can never be reached.
	DEFAULT_MAX_TOOL_CALLS      = 4
	DEFAULT_STANDARD_TOOL_CALLS = 8
	MIDDLEWARE_NONE         = "none"
	MIDDLEWARE_DEFAULT      = "default"
	MIDDLEWARE_SECURE       = "secure"
	MEMORY_STORE_NONE       = "none"
	MEMORY_STORE_FILE       = "file"
	MEMORY_COMPACTION_NONE  = "none"
	MEMORY_COMPACTION_HEAD  = "headline"
	SAFETY_FALLBACK_NONE    = "none"
	SAFETY_FALLBACK_AUTONOM = "autonomy"
	OUTPUT_TEXT             = "text"
	OUTPUT_JSON             = "json"
	OUTPUT_TUI              = "tui"
	SOURCE_FILES            = "files"
	SOURCE_SKILLS           = "skills"
	SOURCE_ENV              = "env"
	SOURCE_REMINDER         = "reminder"
)

// tierRank orders the ladder so Expand can ask "is this tier at least
// standard?" instead of listing names at every branch.
var tierRank = map[string]int{
	TIER_ONESHOT:  0,
	TIER_BASIC:    1,
	TIER_STANDARD: 2,
	TIER_FULL:     3,
}

// Tiers returns the ladder in ascending order.
func Tiers() []string { return []string{TIER_ONESHOT, TIER_BASIC, TIER_STANDARD, TIER_FULL} }

// Rank reports the tier's position on the ladder, and whether the name is
// known at all.
func Rank(tier string) (int, bool) {
	if tier == "" {
		tier = DEFAULT_TIER
	}
	r, ok := tierRank[tier]
	return r, ok
}

// Expand fills in everything the tier implies, then returns the result.
//
// Two rules govern it, and they are the whole contract:
//
//	explicit wins — a block the caller set is never overwritten, only
//	                its empty variant fields are defaulted
//	monotonic     — a higher tier turns on a superset of the lower one
//
// Expand does not validate. Call Validate afterwards (or use Prepare,
// which does both in the right order).
func (c Config) Expand() (Config, error) {
	out := c.Clone()
	if out.Tier == "" {
		out.Tier = DEFAULT_TIER
	}
	rank, ok := Rank(out.Tier)
	if !ok {
		return Config{}, fmt.Errorf("spec: unknown tier %q (want one of %v)", out.Tier, Tiers())
	}

	if out.Model.Provider == "" {
		out.Model.Provider = DEFAULT_PROVIDER
	}

	// --- reasoning: available at every tier, orthogonal to the ladder ---
	if out.Reasoning.Style == "" {
		out.Reasoning.Style = DEFAULT_STYLE
	}
	if len(out.Reasoning.Enable) == 0 {
		out.Reasoning.Enable = []string{out.Reasoning.Style}
	}

	// --- limits ---
	if out.Limits.Autonomy == "" {
		out.Limits.Autonomy = DEFAULT_AUTONOMY
	}
	if out.Limits.MaxTurns == 0 {
		switch {
		case rank == tierRank[TIER_ONESHOT]:
			out.Limits.MaxTurns = DEFAULT_ONESHOT_TURNS
		case rank >= tierRank[TIER_STANDARD]:
			out.Limits.MaxTurns = DEFAULT_STANDARD_TURNS
		default:
			out.Limits.MaxTurns = DEFAULT_MAX_TURNS
		}
	}
	if out.Limits.MaxRounds == 0 {
		switch {
		case rank == tierRank[TIER_ONESHOT]:
			out.Limits.MaxRounds = DEFAULT_ONESHOT_ROUNDS
		case rank >= tierRank[TIER_STANDARD]:
			out.Limits.MaxRounds = DEFAULT_STANDARD_ROUNDS
		default:
			out.Limits.MaxRounds = DEFAULT_MAX_ROUNDS
		}
		// A caller who set only MaxTurns must still get a round default
		// that fits under it. Otherwise the tier default could exceed an
		// explicit turn ceiling and Validate would reject a pairing the
		// caller never wrote.
		if out.Limits.MaxTurns > 0 {
			out.Limits.MaxRounds = min(out.Limits.MaxRounds, out.Limits.MaxTurns)
		}
	}
	// TIER_ONESHOT is deliberately absent: it registers no tools, so a
	// per-round tool-call ceiling there would be dead configuration.
	if out.Limits.MaxToolCalls == 0 && rank > tierRank[TIER_ONESHOT] {
		if rank >= tierRank[TIER_STANDARD] {
			out.Limits.MaxToolCalls = DEFAULT_STANDARD_TOOL_CALLS
		} else {
			out.Limits.MaxToolCalls = DEFAULT_MAX_TOOL_CALLS
		}
	}

	// --- T1 basic: middleware + persistence ---
	// TIER_ONESHOT deliberately leaves both nil. Turning on the store
	// would make Name required at the cheapest tier, and would write
	// state/WAL/log into ~/.config on what callers treat as a plain
	// function call. One line (`memory: {}`) opts back in.
	if rank >= tierRank[TIER_BASIC] {
		if out.Middleware == nil {
			out.Middleware = &Middleware{}
		}
		if out.Memory == nil {
			out.Memory = &Memory{}
		}
	}

	// --- T2 standard: tools, safety, prompt sources, sessions ---
	if rank >= tierRank[TIER_STANDARD] {
		if out.Tools == nil {
			out.Tools = &Tools{}
		}
		if out.Safety == nil {
			out.Safety = &Safety{Sandbox: true}
		}
		if out.Prompt == nil {
			out.Prompt = &Prompt{}
		}
		if out.Sessions == nil {
			out.Sessions = &Sessions{}
		}
	}

	// --- T3 full: skills, subagents, richer prompt sources ---
	if rank >= tierRank[TIER_FULL] {
		if out.Skills == nil {
			out.Skills = &Skills{}
		}
		if out.Subagents == nil {
			out.Subagents = &Subagents{}
		}
		if out.Output == nil {
			out.Output = &Output{}
		}
	}

	// --- implied blocks ---
	// A skill index only reaches the model through the prompt, so asking
	// for skills implies the prompt block. Filling this in beats making
	// the caller learn the plumbing — which is the whole point of the
	// declarative layer. The matching prompt source is added below, once
	// Sources has been defaulted.
	if out.Skills != nil && out.Prompt == nil {
		out.Prompt = &Prompt{}
	}

	applyBlockDefaults(&out, rank)
	return out, nil
}

// applyBlockDefaults fills empty variant fields inside blocks that are on.
// It runs for blocks the caller enabled explicitly too — that is the
// "explicit wins" rule at work: the caller chose to enable the block, we
// only supply the variants they left blank.
func applyBlockDefaults(c *Config, rank int) {
	if c.Middleware != nil && c.Middleware.Preset == "" {
		// Secure once tools exist: sandbox and approval only mean
		// something when there is something to gate.
		if rank >= tierRank[TIER_STANDARD] {
			c.Middleware.Preset = MIDDLEWARE_SECURE
		} else {
			c.Middleware.Preset = MIDDLEWARE_DEFAULT
		}
	}
	if c.Memory != nil {
		if c.Memory.Store == "" {
			c.Memory.Store = MEMORY_STORE_FILE
		}
		if c.Memory.Compaction == "" {
			c.Memory.Compaction = MEMORY_COMPACTION_NONE
		}
	}
	if c.Safety != nil {
		if c.Safety.Mode == "" {
			c.Safety.Mode = MODE_DEFAULT
		}
		if c.Safety.Fallback == "" {
			c.Safety.Fallback = SAFETY_FALLBACK_AUTONOM
		}
	}
	if c.Prompt != nil {
		if c.Prompt.ProjectDir == "" {
			c.Prompt.ProjectDir = DEFAULT_PROJECT_DIR
		}
		if len(c.Prompt.Sources) == 0 {
			if rank >= tierRank[TIER_FULL] {
				c.Prompt.Sources = []string{SOURCE_FILES, SOURCE_SKILLS, SOURCE_ENV, SOURCE_REMINDER}
			} else {
				c.Prompt.Sources = []string{SOURCE_FILES}
			}
		}
		if c.Skills != nil && !slices.Contains(c.Prompt.Sources, SOURCE_SKILLS) {
			c.Prompt.Sources = append(c.Prompt.Sources, SOURCE_SKILLS)
		}
	}
	if c.Subagents != nil {
		if c.Subagents.MaxDepth == 0 {
			c.Subagents.MaxDepth = DEFAULT_SUBAGENT_DEPTH
		}
		if c.Subagents.MaxTurns == 0 {
			c.Subagents.MaxTurns = min(c.Limits.MaxTurns, DEFAULT_SUBAGENT_TURNS)
		}
	}
	if c.Output != nil && c.Output.Format == "" {
		c.Output.Format = OUTPUT_TEXT
	}
	if c.Telemetry != nil && c.Telemetry.Service == "" {
		c.Telemetry.Service = c.Name
	}
}
