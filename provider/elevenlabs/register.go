package elevenlabs

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *SpeechProvider satisfies both speech capabilities.
var (
	_ provider.SpeechGenerator = (*SpeechProvider)(nil)
	_ provider.SpeechStreamer  = (*SpeechProvider)(nil)
)

// Compile-time: ensure *TranscribeProvider satisfies provider.Transcriber.
var _ provider.Transcriber = (*TranscribeProvider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "elevenlabs",
		Metadata: provider.Metadata{
			Label:              "ElevenLabs",
			Note:               "audio only; no chat surface",
			APIKeyEnv:          []string{APIKeyEnvVar},
			BaseURLEnv:         BaseURLEnvVar,
			CredentialRequired: true,
		},
		NewSpeech: func(cfg provider.ResolvedConfig) (provider.SpeechGenerator, error) {
			return NewSpeech(cfg)
		},
		NewTranscriber: func(cfg provider.ResolvedConfig) (provider.Transcriber, error) {
			return NewTranscriber(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
