package provider_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalCapabilityVocabulary(t *testing.T) {
	t.Parallel()

	assert.Equal(t, provider.Capability("chat"), provider.CAPABILITY_CHAT)
	assert.Equal(t, provider.Capability("catalog"), provider.CAPABILITY_CATALOG)
	assert.Equal(t, provider.Capability("image"), provider.CAPABILITY_IMAGE)
	assert.Equal(t, provider.Capability("video"), provider.CAPABILITY_VIDEO)
	assert.Equal(t, provider.Capability("music"), provider.CAPABILITY_MUSIC)
	assert.Equal(t, provider.Capability("transcribe"), provider.CAPABILITY_TRANSCRIBE)
	assert.Equal(t, provider.Capability("speech"), provider.CAPABILITY_SPEECH)
	assert.Equal(t, provider.Capability("live"), provider.CAPABILITY_LIVE)
	assert.Equal(t, provider.Capability("translate"), provider.CAPABILITY_TRANSLATE)
}

func TestAudioCapabilitiesReportTheirFactories(t *testing.T) {
	t.Parallel()

	transcriber := func(provider.ResolvedConfig) (provider.Transcriber, error) {
		return nil, nil
	}
	speech := func(provider.ResolvedConfig) (provider.SpeechGenerator, error) {
		return nil, nil
	}

	tests := []struct {
		name  string
		entry provider.Entry
		want  []provider.Capability
	}{
		{
			name:  "no audio factories",
			entry: provider.Entry{Name: "silent"},
		},
		{
			name:  "transcribe only",
			entry: provider.Entry{Name: "ears", NewTranscriber: transcriber},
			want:  []provider.Capability{provider.CAPABILITY_TRANSCRIBE},
		},
		{
			name:  "speech only",
			entry: provider.Entry{Name: "voice", NewSpeech: speech},
			want:  []provider.Capability{provider.CAPABILITY_SPEECH},
		},
		{
			name: "both, in declaration order",
			entry: provider.Entry{
				Name:           "audio",
				NewTranscriber: transcriber,
				NewSpeech:      speech,
			},
			want: []provider.Capability{
				provider.CAPABILITY_TRANSCRIBE,
				provider.CAPABILITY_SPEECH,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.entry.NewTranscriber != nil,
				tt.entry.Supports(provider.CAPABILITY_TRANSCRIBE))
			assert.Equal(t, tt.entry.NewSpeech != nil,
				tt.entry.Supports(provider.CAPABILITY_SPEECH))
			assert.Equal(t, tt.want, audioCapabilities(tt.entry.Capabilities()))
		})
	}
}

func TestCapabilitiesFollowCanonicalOrder(t *testing.T) {
	// Capabilities() feeds --list output; the order is a rendering contract.
	entry := provider.Entry{
		Name: "everything",
		New: func(provider.ResolvedConfig) (provider.Adapter, error) {
			return nil, nil
		},
		NewImage: func(provider.ResolvedConfig) (provider.ImageGenerator, error) {
			return nil, nil
		},
		NewVideo: func(provider.ResolvedConfig) (provider.VideoGenerator, error) {
			return nil, nil
		},
		NewMusic: func(provider.ResolvedConfig) (provider.MusicGenerator, error) {
			return nil, nil
		},
		NewTranscriber: func(provider.ResolvedConfig) (provider.Transcriber, error) {
			return nil, nil
		},
		NewSpeech: func(provider.ResolvedConfig) (provider.SpeechGenerator, error) {
			return nil, nil
		},
		NewLive: func(provider.ResolvedConfig) (provider.LiveConnector, error) {
			return nil, nil
		},
		NewTranslate: func(provider.ResolvedConfig) (provider.Translator, error) {
			return nil, nil
		},
		Catalog: func() []provider.ModelSpec { return nil },
	}
	assert.Equal(t, []provider.Capability{
		provider.CAPABILITY_CHAT,
		provider.CAPABILITY_CATALOG,
		provider.CAPABILITY_IMAGE,
		provider.CAPABILITY_VIDEO,
		provider.CAPABILITY_MUSIC,
		provider.CAPABILITY_TRANSCRIBE,
		provider.CAPABILITY_SPEECH,
		provider.CAPABILITY_LIVE,
		provider.CAPABILITY_TRANSLATE,
	}, entry.Capabilities())
}

func TestRegisteredAudioCapabilitiesAreExplicit(t *testing.T) {
	elevenlabs, ok := provider.Lookup("elevenlabs")
	require.True(t, ok)
	assert.True(t, elevenlabs.Supports(provider.CAPABILITY_TRANSCRIBE))
	assert.True(t, elevenlabs.Supports(provider.CAPABILITY_SPEECH))
	assert.False(t, elevenlabs.Supports(provider.CAPABILITY_CHAT),
		"elevenlabs has no chat surface")

	minimax, ok := provider.Lookup("minimax")
	require.True(t, ok)
	assert.True(t, minimax.Supports(provider.CAPABILITY_SPEECH))
	assert.False(t, minimax.Supports(provider.CAPABILITY_TRANSCRIBE))

	for _, name := range []string{
		"anthropic", "antigravity", "codex", "google", "grok", "ollama",
	} {
		t.Run(name, func(t *testing.T) {
			entry, ok := provider.Lookup(name)
			require.True(t, ok)
			assert.Empty(t, audioCapabilities(entry.Capabilities()))
		})
	}
}

func TestRegisterAcceptsAnAudioOnlyEntry(t *testing.T) {
	// ElevenLabs is the first entry with New == nil; an audio factory alone
	// must be enough to reach the registry.
	require.NotPanics(t, func() {
		provider.Register(provider.Entry{
			Name: "audio-only-registration",
			Metadata: provider.Metadata{
				Label:     "Audio Only",
				APIKeyEnv: []string{"AUDIO_ONLY_API_KEY"},
			},
			NewSpeech: func(provider.ResolvedConfig) (provider.SpeechGenerator, error) {
				return nil, nil
			},
		})
	})

	entry, ok := provider.Lookup("audio-only-registration")
	require.True(t, ok)
	assert.Nil(t, entry.New)
	assert.Equal(t,
		[]provider.Capability{provider.CAPABILITY_SPEECH},
		entry.Capabilities())

	_, err := provider.New("audio-only-registration", provider.Options{
		LookupEnv: func(string) string { return "" },
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrUnsupportedCapability,
		"an audio-only entry must still refuse the model capability by name")
}

func audioCapabilities(capabilities []provider.Capability) []provider.Capability {
	var out []provider.Capability
	for _, capability := range capabilities {
		switch capability {
		case provider.CAPABILITY_TRANSCRIBE, provider.CAPABILITY_SPEECH:
			out = append(out, capability)
		}
	}
	return out
}
