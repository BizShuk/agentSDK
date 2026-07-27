package grok

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)
var _ provider.ImageGenerator = (*Provider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "grok",
		Metadata: provider.Metadata{
			Label:              "xAI Grok",
			APIKeyEnv:          []string{APIKeyEnvVar},
			OAuthEnv:           []string{OAuthEnvVar},
			BaseURLEnv:         BaseURLEnvVar,
			CredentialRequired: true,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		NewImage: func(cfg provider.ResolvedConfig) (provider.ImageGenerator, error) {
			return NewImage(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
