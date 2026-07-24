package agent

import (
	"fmt"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	anthropicprovider "github.com/bizshuk/agentsdk/provider/anthropic"
	antigravityprovider "github.com/bizshuk/agentsdk/provider/antigravity"
	codexprovider "github.com/bizshuk/agentsdk/provider/codex"
	googleprovider "github.com/bizshuk/agentsdk/provider/google"
	grokprovider "github.com/bizshuk/agentsdk/provider/grok"
	minimaxprovider "github.com/bizshuk/agentsdk/provider/minimax"
	ollamaprovider "github.com/bizshuk/agentsdk/provider/ollama"
)

// ModelChoices returns the model choices for a given provider by reading
// the provider's bundled DefaultCatalog().
func ModelChoices(provider string) []Choice {
	key := strings.ToLower(strings.TrimSpace(provider))
	var specs []core.ModelSpec

	switch key {
	case "anthropic":
		specs = anthropicprovider.DefaultCatalog()
	case "google":
		specs = googleprovider.DefaultCatalog()
	case "grok":
		specs = grokprovider.DefaultCatalog()
	case "ollama":
		specs = ollamaprovider.DefaultCatalog()
	case "codex":
		specs = codexprovider.DefaultCatalog()
	case "antigravity":
		specs = antigravityprovider.DefaultCatalog()
	case "minimax", "":
		specs = minimaxprovider.DefaultCatalog()
	default:
		return nil
	}

	out := make([]Choice, 0, len(specs)+1)
	out = append(out, Choice{
		Value:   "",
		Label:   "(default)",
		Note:    "the adapter's flagship default",
		Default: true,
	})

	for _, s := range specs {
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

		out = append(out, Choice{
			Value: s.ID,
			Label: s.ID,
			Note:  strings.Join(parts, "; "),
		})
	}
	return out
}
