package google

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)
var _ provider.ImageGenerator = (*Provider)(nil)
var _ provider.LiveConnector = (*LiveProvider)(nil)
var _ provider.Translator = (*TranslateProvider)(nil)
var _ provider.TranslateStreamer = (*TranslateProvider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "google",
		Metadata: provider.Metadata{
			Label:              "Google Gemini",
			APIKeyEnv:          []string{APIKeyEnvVar},
			BaseURLEnv:         BaseURLEnvVar,
			LiveBaseURLEnv:     LiveBaseURLEnvVar,
			CredentialRequired: true,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		NewImage: func(cfg provider.ResolvedConfig) (provider.ImageGenerator, error) {
			return NewImage(cfg)
		},
		NewLive: func(cfg provider.ResolvedConfig) (provider.LiveConnector, error) {
			return NewLive(cfg)
		},
		NewTranslate: func(cfg provider.ResolvedConfig) (provider.Translator, error) {
			return NewTranslate(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
