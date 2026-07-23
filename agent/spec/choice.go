package spec

import "github.com/bizshuk/agentsdk/core"

// Permission modes. Duplicated as literals rather than imported from the
// permission package: spec must stay dependency-free so a wizard or schema
// tool can read it without pulling the harness in. The strings are the
// config vocabulary; permission.Mode is the runtime type, and agent maps
// between them.
const (
	MODE_DEFAULT      = "default"
	MODE_ACCEPT_EDITS = "acceptEdits"
	MODE_PLAN         = "plan"
	MODE_BYPASS       = "bypassPermissions"
)

// Built-in tool names, matching tool.RegisterDefaults.
const (
	TOOL_READ  = "read"
	TOOL_WRITE = "write"
	TOOL_EDIT  = "edit"
	TOOL_BASH  = "bash"
	TOOL_GLOB  = "glob"
	TOOL_GREP  = "grep"
)

// Choice is one selectable value for a config field, plus enough metadata
// to render it in a wizard, a form, or a --list listing.
//
// Choice is DATA, deliberately: a wizard has to enumerate the candidates
// before anything is applied, and the result crosses a serialization
// boundary — it gets written to a config file and read back by a later
// process. That is the opposite of agent.Option, which is an opaque
// closure applied inside one process and cannot describe itself.
type Choice struct {
	Value   string `json:"value"`
	Label   string `json:"label,omitempty"`
	Note    string `json:"note,omitempty"`
	Default bool   `json:"default,omitempty"`
}

// TierChoices lists the engagement ladder.
func TierChoices() []Choice {
	return []Choice{
		{Value: TIER_ONESHOT, Label: "one-shot", Note: "single model call; no tools, no persistence"},
		{Value: TIER_BASIC, Label: "basic", Default: true, Note: "reasoning loop + middleware + state/WAL"},
		{Value: TIER_STANDARD, Label: "standard", Note: "+ built-in tools, permission, sessions, context files"},
		{Value: TIER_FULL, Label: "full", Note: "+ skills, subagents, stream output"},
	}
}

// StyleChoices lists the reasoning strategies.
//
// The values come from core's ReasoningStyle constants rather than from
// the planning package: enumerating a style needs only the name, while
// constructing its rule needs planning — so spec stays core-only.
func StyleChoices() []Choice {
	return []Choice{
		{Value: string(core.REASON_REACT), Label: "think then act", Default: true,
			Note: "reason → tool → observe → repeat (ReAct)"},
		{Value: string(core.REASON_PLAN_THEN_RUN), Label: "plan then run",
			Note: "blueprint first, then execute each step"},
		{Value: string(core.REASON_DO_THEN_REVIEW), Label: "do then review",
			Note: "execute, then critique and iterate"},
		{Value: string(core.REASON_ONE_SHOT), Label: "one shot",
			Note: "single chain-of-thought call, then done"},
		{Value: string(core.REASON_LEARN_FROM_FAILURE), Label: "learn from failure",
			Note: "retry with reflection over past failures"},
		{Value: string(core.REASON_PICK_AGENT), Label: "choose agent",
			Note: "router — register the styles it may delegate to in enable[]"},
	}
}

// VariantChoices returns the candidates for a block's variant field.
// The key is "<block>.<field>" so one function serves every block and a
// wizard can drive its stages from a table instead of a switch.
func VariantChoices(key string) []Choice {
	switch key {
	case "middleware.preset":
		return []Choice{
			{Value: MIDDLEWARE_DEFAULT, Label: "default", Default: true,
				Note: "retry → timeout → budget → loopguard"},
			{Value: MIDDLEWARE_SECURE, Label: "secure",
				Note: "default + sandbox → approval → spotlight → sanitizer"},
			{Value: MIDDLEWARE_NONE, Label: "none", Note: "no-op chain"},
		}
	case "memory.store":
		return []Choice{
			{Value: MEMORY_STORE_FILE, Label: "file", Default: true,
				Note: "JSON state + JSONL WAL under ~/.config/<name>/data; enables resume"},
			{Value: MEMORY_STORE_NONE, Label: "none", Note: "in-memory only; no resume, no crash recovery"},
		}
	case "memory.compaction":
		return []Choice{
			{Value: MEMORY_COMPACTION_NONE, Label: "none", Default: true},
			{Value: MEMORY_COMPACTION_HEAD, Label: "headline", Note: "summarize older turns when the window fills"},
		}
	case "safety.mode":
		return []Choice{
			{Value: MODE_DEFAULT, Label: "default", Default: true, Note: "rules first, then the fallback grid"},
			{Value: MODE_ACCEPT_EDITS, Label: "accept edits", Note: "low-risk auto-allowed, high-risk still asks"},
			{Value: MODE_PLAN, Label: "plan", Note: "read-only: high-risk denied outright"},
			{Value: MODE_BYPASS, Label: "bypass", Note: "everything allowed — containers / CI only"},
		}
	case "safety.fallback":
		return []Choice{
			{Value: SAFETY_FALLBACK_AUTONOM, Label: "autonomy grid", Default: true,
				Note: "fall back to the L0-L4 risk grid when no rule matches"},
			{Value: SAFETY_FALLBACK_NONE, Label: "none", Note: "unmatched calls are allowed"},
		}
	case "output.format":
		return []Choice{
			{Value: OUTPUT_TEXT, Label: "text", Default: true, Note: "plain stdout"},
			{Value: OUTPUT_JSON, Label: "json", Note: "stream-json envelopes (wire)"},
			{Value: OUTPUT_TUI, Label: "tui", Note: "interactive terminal UI"},
		}
	case "prompt.sources":
		return []Choice{
			{Value: SOURCE_FILES, Label: "context files", Default: true,
				Note: "AGENTS.md / CLAUDE.md hierarchy"},
			{Value: SOURCE_SKILLS, Label: "skill index", Note: "progressive-disclosure skill listing"},
			{Value: SOURCE_ENV, Label: "environment", Note: "cwd, date, git branch"},
			{Value: SOURCE_REMINDER, Label: "per-turn reminder", Note: "re-injected every turn"},
		}
	case "tools.builtin":
		return []Choice{
			{Value: TOOL_READ, Label: "read", Default: true},
			{Value: TOOL_WRITE, Label: "write", Default: true},
			{Value: TOOL_EDIT, Label: "edit", Default: true},
			{Value: TOOL_BASH, Label: "bash", Default: true, Note: "highest risk — gate it with safety rules"},
			{Value: TOOL_GLOB, Label: "glob", Default: true},
			{Value: TOOL_GREP, Label: "grep", Default: true},
		}
	case "limits.autonomy":
		return []Choice{
			{Value: "L0", Label: "L0", Note: "every tool call needs approval"},
			{Value: "L1", Label: "L1"},
			{Value: "L2", Label: "L2", Default: true, Note: "low-risk automatic, high-risk asks"},
			{Value: "L3", Label: "L3"},
			{Value: "L4", Label: "L4", Note: "fully autonomous"},
		}
	default:
		return nil
	}
}

// VariantKeys lists every key VariantChoices answers to, in the order a
// wizard should walk them. Keeping the order here rather than in the
// wizard means adding a variant does not require touching the CLI.
func VariantKeys() []string {
	return []string{
		"middleware.preset",
		"memory.store",
		"memory.compaction",
		"tools.builtin",
		"safety.mode",
		"safety.fallback",
		"prompt.sources",
		"limits.autonomy",
		"output.format",
	}
}

// Values flattens a choice list to its values — the allowlist form used
// by validation.
func Values(cs []Choice) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Value)
	}
	return out
}

// DefaultOf returns the choice marked Default, or "" when a list has none
// (a multi-select like prompt.sources may mark several).
func DefaultOf(cs []Choice) string {
	for _, c := range cs {
		if c.Default {
			return c.Value
		}
	}
	return ""
}
