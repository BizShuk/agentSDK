package antigravity

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter and the
// optional live-catalog capability, and that the image capability is
// reachable through the registry's shared contract.
var (
	_ provider.Adapter        = (*Provider)(nil)
	_ provider.ModelLister    = (*Provider)(nil)
	_ provider.ImageGenerator = (*ImageProvider)(nil)
)

func init() {
	provider.Register(provider.Entry{
		Name: "antigravity",
		Metadata: provider.Metadata{
			Label: "Antigravity",
			Note:  "Google Cloud Code v1internal gateway (Gemini + Claude), OAuth only",
			// No APIKeyEnv: the gateway takes a Google OAuth access
			// token and nothing else, so an api_key credential kind is
			// rejected during resolution rather than sent and refused
			// upstream. An explicitly supplied Options.APIKey still
			// reaches the adapter, for deployments fronted by a local
			// proxy.
			OAuthEnv:           []string{OAuthEnvVar},
			BaseURLEnv:         BaseURLEnvVar,
			CredentialRequired: true,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		// No ImageBaseURLEnv: image generation runs on the same
		// v1internal host as chat, so it resolves the same base URL.
		NewImage: func(cfg provider.ResolvedConfig) (provider.ImageGenerator, error) {
			return NewImageGenerator(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
