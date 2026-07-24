package anthropic

import (
	"github.com/bizshuk/agentsdk/provider"
)

// adapterMetadata is the single source of truth for anthropic's
// registration descriptor. Both Entry.Metadata and *Provider.Metadata()
// return this same value; the function form keeps APIKeyEnv immutable
// across calls (a fresh slice each time, so external mutation cannot
// bleed into the registered descriptor).
func adapterMetadata() registry.Metadata {
	return registry.Metadata{
		Label:     "Anthropic",
		Note:      "OAuth token outranks API key",
		APIKeyEnv: []string{"ANTHROPIC_OAUTH_TOKEN", "ANTHROPIC_API_KEY"},
	}
}

// Compile-time: ensure *Provider satisfies registry.Adapter.
var _ registry.Adapter = (*Provider)(nil)

func init() {
	meta := adapterMetadata()
	registry.Register(registry.Entry{
		Name:     "anthropic",
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
