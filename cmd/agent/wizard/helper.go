package wizard

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/provider"
)

func detectDefaultPersona() string {
	if _, err := os.Stat("CLAUDE.md"); err == nil {
		return "CLAUDE.md"
	}
	if _, err := os.Stat("AGENTS.md"); err == nil {
		return "AGENTS.md"
	}
	return ""
}

func rankOf(tier string) int {
	r, _ := spec.Rank(tier)
	return r
}

func orDefault(got, fallback string) string {
	if got != "" {
		return got
	}
	return fallback
}

// listChoices prints one field's candidates.
func listChoices(out io.Writer, key string) error {
	var choices []spec.Choice
	switch key {
	case "tier":
		choices = spec.TierChoices()
	case "reasoning.style", "reasoning.enable":
		choices = spec.StyleChoices()
	case "model.provider":
		choices = providerChoices(provider.Entries())
	default:
		choices = spec.VariantChoices(key)
	}
	if len(choices) == 0 {
		return fmt.Errorf("unknown field %q (try: tier, model.provider, reasoning.style, %s)",
			key, strings.Join(spec.VariantKeys(), ", "))
	}
	for _, c := range choices {
		mark := " "
		if c.Default {
			mark = "*"
		}
		fmt.Fprintf(out, "%s %-18s %s\n", mark, c.Value, c.Note)
	}
	return nil
}

// goLiteral renders the config as Go source.
func goLiteral(cfg agent.Config) string {
	var b strings.Builder
	b.WriteString("agent.Main(agent.MustNew(agent.Config{\n")
	fmt.Fprintf(&b, "\tName: %q,\n", cfg.Name)
	fmt.Fprintf(&b, "\tTier: %q,\n", cfg.Tier)
	if cfg.Persona != "" {
		fmt.Fprintf(&b, "\tPersona: %q,\n", cfg.Persona)
	}
	fmt.Fprintf(&b, "\tModel: agent.Model{Provider: %q", cfg.Model.Provider)
	if cfg.Model.Name != "" {
		fmt.Fprintf(&b, ", Name: %q", cfg.Model.Name)
	}
	b.WriteString("},\n")
	fmt.Fprintf(&b, "\tReasoning: agent.Reasoning{Style: %q},\n", cfg.Reasoning.Style)
	fmt.Fprintf(&b, "\tLimits: agent.Limits{MaxTurns: %d, Autonomy: %q},\n",
		cfg.Limits.MaxTurns, cfg.Limits.Autonomy)
	if cfg.Safety != nil {
		fmt.Fprintf(&b, "\tSafety: &agent.Safety{Mode: %q, Sandbox: %v},\n",
			cfg.Safety.Mode, cfg.Safety.Sandbox)
	}
	b.WriteString("}))\n")
	return b.String()
}
