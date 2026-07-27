package anthropic

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "anthropic",
		Metadata: provider.Metadata{
			Label:              "Anthropic",
			Note:               "OAuth token outranks API key",
			OAuthEnv:           []string{APIKeyOAuthEnvVar},
			APIKeyEnv:          []string{APIKeyEnvVar},
			CredentialRequired: true,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
