package codex

import (
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

func init() {
	registry.Register(registry.Entry{
		Name:       "codex",
		Label:      "Codex",
		Note:       "OpenAI Codex Responses via OAuth",
		BaseURLEnv: BaseURLEnvVar,
		New: func(o registry.Options) (core.Provider, error) {
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