package minimax

import (
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

func init() {
	registry.Register(registry.Entry{
		Name:       "minimax",
		Label:      "MiniMax",
		Note:       "default; OpenAI-compatible",
		APIKeyEnv:  []string{"MINIMAX_API_KEY"},
		BaseURLEnv: "MINIMAX_BASE_URL",
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