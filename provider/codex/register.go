package codex

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "codex",
		Metadata: provider.Metadata{
			Label:              "Codex",
			Note:               "OpenAI Codex Responses via credential decorator",
			APIKeyEnv:          []string{APIKeyEnvVar},
			OAuthEnv:           []string{OAuthEnvVar},
			BaseURLEnv:         BaseURLEnvVar,
			CredentialRequired: true,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
