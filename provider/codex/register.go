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
			Label:      "Codex",
			Note:       "OpenAI Codex Responses via OAuth (constructor path)",
			BaseURLEnv: BaseURLEnvVar,
		},
		New: func(o provider.Options) (provider.Adapter, error) {
			var opts []Option
			if o.Model != "" {
				opts = append(opts, WithModel(o.Model))
			}
			if o.APIKey != "" {
				opts = append(opts, WithAPIKey(o.APIKey))
			}
			if o.BaseURL != "" {
				opts = append(opts, WithBaseURL(o.BaseURL))
			}
			return New(opts...)
		},
		Catalog: DefaultCatalog,
	})
}
