package codex

import (
	"github.com/bizshuk/agentsdk/provider"
)

// adapterMetadata is the single source of truth for codex's
// registration descriptor. Both Entry.Metadata and *Provider.Metadata()
// return this same value; the function form keeps APIKeyEnv immutable
// across calls. codex is OAuth-first; NewWithOAuth is the production
// path. The New() API-key route exists for tests only.
func adapterMetadata() registry.Metadata {
	return registry.Metadata{
		Label:      "Codex",
		Note:       "OpenAI Codex Responses via OAuth",
		BaseURLEnv: BaseURLEnvVar,
	}
}

// Compile-time: ensure *Provider satisfies registry.Adapter.
var _ registry.Adapter = (*Provider)(nil)

func init() {
	meta := adapterMetadata()
	registry.Register(registry.Entry{
		Name:     "codex",
		Metadata: meta,
		New: func(o registry.Options) (registry.Adapter, error) {
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
