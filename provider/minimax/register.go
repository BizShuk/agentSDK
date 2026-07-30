package minimax

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)

// Compile-time: ensure *VideoProvider satisfies provider.VideoGenerator.
var _ provider.VideoGenerator = (*VideoProvider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "minimax",
		Metadata: provider.Metadata{
			Label:              "MiniMax",
			Note:               "default; OpenAI-compatible",
			APIKeyEnv:          []string{APIKeyEnvVar},
			OAuthEnv:           []string{OAuthEnvVar},
			BaseURLEnv:         BaseURLEnvVar,
			VideoBaseURLEnv:    VideoBaseURLEnvVar,
			CredentialRequired: true,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		NewVideo: func(cfg provider.ResolvedConfig) (provider.VideoGenerator, error) {
			return NewVideo(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
