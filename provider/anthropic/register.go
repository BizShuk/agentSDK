package anthropic

import (
	"github.com/bizshuk/agentsdk/provider"
)

// adapterMetadata is the single source of truth for anthropic's
// registration descriptor. Both Entry.Metadata and *Provider.Metadata()
// return this same value; the function form keeps APIKeyEnv immutable
// across calls (a fresh slice each time, so external mutation cannot
// bleed into the registered descriptor).
func adapterMetadata() provider.Metadata {
	return provider.Metadata{
		Label:     "Anthropic",
		Note:      "OAuth token outranks API key",
		OAuthEnv:  []string{"ANTHROPIC_OAUTH_TOKEN"},
		APIKeyEnv: []string{"ANTHROPIC_API_KEY"},
	}
}

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)

func init() {
	meta := adapterMetadata()
	provider.Register(provider.Entry{
		Name:     "anthropic",
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
