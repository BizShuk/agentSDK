package agent

import (
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

// ModelChoices returns the model choices for a given provider by reading
// the provider's bundled catalog through the registry. The registry owns
// the name → adapter mapping, so adding a new adapter here is a one-line
// entry change rather than a second switch statement.
func ModelChoices(provider string) []Choice {
	specs, ok := registry.Catalog(provider)
	if !ok {
		return nil
	}
	return specsToChoices(specs)
}

// specsToChoices maps a slice of core.ModelSpec into the wizard's
// []Choice vocabulary. The empty-Value entry with Default=true lets the
// wizard prompt accept Enter to mean "the adapter's flagship default".
func specsToChoices(specs []core.ModelSpec) []Choice {
	out := make([]Choice, 0, len(specs)+1)
	out = append(out, Choice{
		Value:   "",
		Label:   "(default)",
		Note:    "the adapter's flagship default",
		Default: true,
	})
	for _, s := range specs {
		out = append(out, Choice{
			Value: s.ID,
			Label: s.ID,
			Note:  modelNote(s),
		})
	}
	return out
}

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