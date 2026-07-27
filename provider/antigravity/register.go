package antigravity

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "antigravity",
		Metadata: provider.Metadata{
			Label:              "Antigravity",
			Note:               "Antigravity gateway",
			APIKeyEnv:          []string{APIKeyEnvVar},
			BaseURLEnv:         BaseURLEnvVar,
			CredentialRequired: true,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
