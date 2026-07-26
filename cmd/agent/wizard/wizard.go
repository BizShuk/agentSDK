package wizard

import (
	"bufio"
	"fmt"
	"io"
	"slices"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/utils/agentconfig"
)

// wizard carries the prompt loop's I/O and the non-interactive switch.
type wizard struct {
	in    *bufio.Scanner
	out   io.Writer
	yes   bool
	isTTY bool
}

// run walks the stages (1/9 to 9/9). Each stage may read and write cfg;
// tier runs first because every later stage's visibility depends on it.
func (w *wizard) run(cfg agent.Config) (agent.Config, error) {
	// --- stage 1: tier ---
	w.section("1/9  tier — how much of the SDK to wire")
	cfg.Tier = w.choose("tier", spec.TierChoices(), orDefault(cfg.Tier, spec.DEFAULT_TIER))
	rank, ok := spec.Rank(cfg.Tier)
	if !ok {
		return cfg, fmt.Errorf("unknown tier %q", cfg.Tier)
	}

	// Expand now so every later stage offers the tier's own defaults.
	expanded, err := cfg.Expand()
	if err != nil {
		return cfg, err
	}
	cfg = expanded

	// --- stage 2: model ---
	w.section("2/9  model — which provider answers")
	// No fallback argument: choose() falls back to the choice marked
	// Default, and ProviderChoices marks it from provider.DEFAULT_NAME.
	// Naming the vendor here too would be a second source of truth.
	cfg.Model.Provider = w.choose("provider", agent.ProviderChoices(), cfg.Model.Provider)

	modelChoices := agent.ModelChoices(cfg.Model.Provider)
	if len(modelChoices) > 0 {
		cfg.Model.Name = w.choose("model id", modelChoices, cfg.Model.Name)
	} else {
		cfg.Model.Name = w.ask("model id (empty = the adapter's flagship default)", cfg.Model.Name)
	}

	// CredentialKind is orthogonal to provider selection: even after
	// picking the family, callers may want to force one credential class
	// when both env vars are exported on the same machine.
	cfg.Model.CredentialKind = w.choose(
		"credential kind (core.CREDENTIAL_KIND_*; matches --credential-kind on provider cmd)",
		spec.VariantChoices("model.credential_kind"),
		cfg.Model.CredentialKind,
	)

	// Persona check: fallback to CLAUDE.md, then AGENTS.md
	defaultPersona := detectDefaultPersona()
	if cfg.Persona == "" && defaultPersona != "" {
		cfg.Persona = defaultPersona
	}
	cfg.Persona = w.ask("persona (fixed system identity, optional)", cfg.Persona)

	// --- stage 3: reasoning ---
	w.section("3/9  reasoning — which strategy runs, and which are registered")
	cfg.Reasoning.Style = w.choose("style", spec.StyleChoices(), cfg.Reasoning.Style)
	cfg.Reasoning.Enable = w.chooseMulti(
		"also register (mid-run switching: alternative reasoning strategies available for dynamic runtime switching)",
		spec.StyleChoices(), cfg.Reasoning.Enable)
	if !slices.Contains(cfg.Reasoning.Enable, cfg.Reasoning.Style) {
		cfg.Reasoning.Enable = append(cfg.Reasoning.Enable, cfg.Reasoning.Style)
	}

	// --- stage 4: tools ---
	if rank >= rankOf(spec.TIER_STANDARD) && cfg.Tools != nil {
		w.section("4/9  tools — what the model can do")
		// Pre-select recommended tools based on reasoning style if empty
		if len(cfg.Tools.Builtin) == 0 {
			cfg.Tools.Builtin = spec.Values(spec.VariantChoices("tools.builtin"))
		}
		cfg.Tools.Builtin = w.chooseMulti("built-in tools (none selected = all)",
			spec.VariantChoices("tools.builtin"), cfg.Tools.Builtin)
	}

	// --- stage 5: safety ---
	if cfg.Safety != nil {
		w.section("5/9  safety — who approves, and what may be touched")
		defaultMode := spec.MODE_ACCEPT_EDITS
		if cfg.Safety.Mode == "" || cfg.Safety.Mode == spec.MODE_DEFAULT {
			cfg.Safety.Mode = defaultMode
		}
		cfg.Safety.Mode = w.choose("mode", spec.VariantChoices("safety.mode"), cfg.Safety.Mode)
		cfg.Safety.Sandbox = w.confirm("sandbox path and command arguments", cfg.Safety.Sandbox)
		cfg.Safety.Deny = w.askList("deny rules, comma separated (e.g. bash(sudo:*))", cfg.Safety.Deny)
		cfg.Safety.Ask = w.askList("ask rules, comma separated", cfg.Safety.Ask)
	}

	// --- prompt stage (removed asking prompt sources & project harness dir) ---
	if cfg.Prompt != nil {
		if cfg.Prompt.ProjectDir == "" {
			if cfg.Name != "" {
				cfg.Prompt.ProjectDir = cfg.Name
			}
		}
	}

	// --- stage 6: subagents ---
	if cfg.Subagents != nil {
		w.section("6/9  subagents — delegation")
		cfg.Subagents.MaxTurns = w.askInt("max turns per delegated run", cfg.Subagents.MaxTurns)
		cfg.Subagents.MaxDepth = w.askInt("max nesting depth", cfg.Subagents.MaxDepth)
	}

	// --- stage 7: memory ---
	if cfg.Memory != nil {
		w.section("7/9  memory — persistence and history")
		cfg.Memory.Store = w.choose("state store", spec.VariantChoices("memory.store"), cfg.Memory.Store)
		cfg.Memory.Compaction = w.choose("compaction", spec.VariantChoices("memory.compaction"), cfg.Memory.Compaction)
	}

	// --- stage 8: output and limits ---
	w.section("8/9  output and limits")
	if cfg.Output != nil {
		cfg.Output.Format = w.choose("format", spec.VariantChoices("output.format"), cfg.Output.Format)
	}
	cfg.Limits.MaxTurns = w.askInt("max turns per run", cfg.Limits.MaxTurns)
	cfg.Limits.Autonomy = w.choose("autonomy", spec.VariantChoices("limits.autonomy"), cfg.Limits.Autonomy)

	if cfg.Memory != nil && cfg.Memory.Store == spec.MEMORY_STORE_FILE {
		cfg.Name = w.ask("application name (resolves ~/.config/<name>)", orDefault(cfg.Name, "my-agent"))
		if cfg.Prompt != nil && cfg.Prompt.ProjectDir == "" {
			cfg.Prompt.ProjectDir = cfg.Name
		}
	}

	// --- stage 9: review ---
	if !w.yes {
		w.section("9/9  review")
		body, err := agentconfig.Marshal(cfg, agentconfig.FORMAT_YAML)
		if err != nil {
			return cfg, err
		}
		fmt.Fprintln(w.out, string(body))
	}
	return cfg, nil
}
