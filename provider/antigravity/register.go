package antigravity

import (
	"github.com/bizshuk/agentsdk/provider"
)

// adapterMetadata is the single source of truth for antigravity's
// registration descriptor. Both Entry.Metadata and *Provider.Metadata()
// return this same value; the function form keeps APIKeyEnv immutable
// across calls.
func adapterMetadata() provider.Metadata {
	return provider.Metadata{
		Label:      "Antigravity",
		Note:       "Antigravity gateway; OAuth via constructor, ANTIGRAVITY_API_KEY for the direct path",
		APIKeyEnv:  []string{APIKeyEnvVar},
		BaseURLEnv: BaseURLEnvVar,
	}
}

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)

func init() {
	meta := adapterMetadata()
	provider.Register(provider.Entry{
		Name:     "antigravity",
		Metadata: meta,
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
