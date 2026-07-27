package ollama

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "ollama",
		Metadata: provider.Metadata{
			Label:      "Ollama",
			Note:       "local; keyless by default",
			APIKeyEnv:  []string{APIKeyEnvVar},
			BaseURLEnv: BaseURLEnvVar,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
