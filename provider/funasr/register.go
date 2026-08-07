package funasr

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *TranscribeProvider satisfies provider.Transcriber.
var _ provider.Transcriber = (*TranscribeProvider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "funasr",
		Metadata: provider.Metadata{
			Label:      "FunASR",
			Note:       "local OpenAI-compatible ASR server; keyless by default",
			APIKeyEnv:  []string{APIKeyEnvVar},
			BaseURLEnv: BaseURLEnvVar,
		},
		NewTranscriber: func(cfg provider.ResolvedConfig) (provider.Transcriber, error) {
			return NewTranscriber(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
