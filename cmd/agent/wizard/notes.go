package wizard

import (
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// choiceFromEntry turns a raw provider.Entry (assembly data) into a
// wizard Choice (UI shape). The Note combines the adapter's own note
// with the credential it reads, so a wizard prompt can tell the user
// what to export before they pick.
func choiceFromEntry(e provider.Entry) spec.Choice {
	return spec.Choice{
		Value:   e.Name,
		Label:   e.Metadata.Label,
		Note:    providerNote(e),
		Default: e.Name == provider.DEFAULT_NAME,
	}
}

// providerNote is the human-facing note attached to each provider
// Choice. It is presentation logic — lives next to the wizard, not in
// agent/, because it is only meaningful when the wizard is rendering
// the choice.
func providerNote(e provider.Entry) string {
	var parts []string
	if e.Metadata.Note != "" {
		parts = append(parts, e.Metadata.Note)
	}
	if len(e.Metadata.APIKeyEnv) > 0 {
		parts = append(parts, "reads "+strings.Join(e.Metadata.APIKeyEnv, " or "))
	}
	return strings.Join(parts, "; ")
}

// catalogChoices maps a slice of core.ModelSpec into the wizard's
// []Choice vocabulary. The empty-Value entry with Default=true lets the
// wizard prompt accept Enter to mean "the adapter's flagship default".
func catalogChoices(specs []core.ModelSpec) []spec.Choice {
	out := make([]spec.Choice, 0, len(specs)+1)
	out = append(out, spec.Choice{
		Value:   "",
		Label:   "(default)",
		Note:    "the adapter's flagship default",
		Default: true,
	})
	for _, s := range specs {
		out = append(out, spec.Choice{
			Value: s.ID,
			Label: s.ID,
			Note:  modelNote(s),
		})
	}
	return out
}

// modelNote is the human-facing note attached to each model Choice.
// Living next to the wizard because it is presentation only.
func modelNote(s core.ModelSpec) string {
	var parts []string
	if s.Family != "" {
		parts = append(parts, s.Family)
	}
	if s.Reasoning {
		parts = append(parts, "reasoning")
	}
	if s.ContextWindow > 0 {
		parts = append(parts, fmt.Sprintf("ctx: %d", s.ContextWindow))
	}
	if s.MaxTokens > 0 {
		parts = append(parts, fmt.Sprintf("max: %d", s.MaxTokens))
	}
	return strings.Join(parts, "; ")
}

// providerChoices wraps agent.ProviderEntries() (raw data) into the
// wizard's []Choice. It is the only place that knows the assembly side
// AND the UI side at once, and it is also the seam a future call site
// uses if it wants a different defaulting rule.
func providerChoices(entries []provider.Entry) []spec.Choice {
	out := make([]spec.Choice, 0, len(entries))
	for _, e := range entries {
		out = append(out, choiceFromEntry(e))
	}
	return out
}
