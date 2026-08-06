package minimax

import (
	"github.com/bizshuk/agentsdk/provider"
)

// Compile-time: ensure *Provider satisfies provider.Adapter.
var _ provider.Adapter = (*Provider)(nil)

// Compile-time: ensure *VideoProvider satisfies provider.VideoGenerator.
var _ provider.VideoGenerator = (*VideoProvider)(nil)

// Compile-time: ensure *MusicProvider satisfies provider.MusicGenerator.
var _ provider.MusicGenerator = (*MusicProvider)(nil)

// Compile-time: ensure *SpeechProvider satisfies provider.SpeechGenerator.
var _ provider.SpeechGenerator = (*SpeechProvider)(nil)

func init() {
	provider.Register(provider.Entry{
		Name: "minimax",
		Metadata: provider.Metadata{
			Label:              "MiniMax",
			Note:               "default; OpenAI-compatible",
			APIKeyEnv:          []string{APIKeyEnvVar},
			BaseURLEnv:         BaseURLEnvVar,
			ImageBaseURLEnv:    ImageBaseURLEnvVar,
			VideoBaseURLEnv:    VideoBaseURLEnvVar,
			MusicBaseURLEnv:    MusicBaseURLEnvVar,
			SpeechBaseURLEnv:   SpeechBaseURLEnvVar,
			CredentialRequired: true,
		},
		New: func(cfg provider.ResolvedConfig) (provider.Adapter, error) {
			return New(cfg)
		},
		NewImage: func(cfg provider.ResolvedConfig) (provider.ImageGenerator, error) {
			return NewImage(cfg)
		},
		NewVideo: func(cfg provider.ResolvedConfig) (provider.VideoGenerator, error) {
			return NewVideo(cfg)
		},
		NewMusic: func(cfg provider.ResolvedConfig) (provider.MusicGenerator, error) {
			return NewMusic(cfg)
		},
		NewSpeech: func(cfg provider.ResolvedConfig) (provider.SpeechGenerator, error) {
			return NewSpeech(cfg)
		},
		Catalog: DefaultCatalog,
	})
}
