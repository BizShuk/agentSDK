package grok

import (
	"github.com/bizshuk/agentsdk/provider"
)

// adapterMetadata is the single source of truth for grok's
// registration descriptor. Both Entry.Metadata and *Provider.Metadata()
// return this same value; the function form keeps APIKeyEnv immutable
// across calls.
func adapterMetadata() registry.Metadata {
	return registry.Metadata{
		Label:      "xAI Grok",
		APIKeyEnv:  []string{"XAI_API_KEY"},
		BaseURLEnv: "XAI_BASE_URL",
	}
}

// Compile-time: ensure *Provider satisfies registry.Adapter.
var _ registry.Adapter = (*Provider)(nil)

func init() {
	meta := adapterMetadata()
	registry.Register(registry.Entry{
		Name:     "grok",
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
