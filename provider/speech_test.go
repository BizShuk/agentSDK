package provider_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingSpeechGenerator struct {
	seen  []core.Auth
	calls int
}

func (r *recordingSpeechGenerator) GenerateSpeech(
	_ context.Context,
	req provider.SpeechRequest,
) (provider.SpeechResult, error) {
	r.calls++
	r.seen = append(r.seen, req.Auth)
	return provider.SpeechResult{
		Audio: provider.SpeechAsset{Bytes: []byte("audio"), Format: "mp3"},
	}, nil
}

// recordingSpeechStreamer is the ElevenLabs shape: one type implementing both
// the blocking and the streaming capability.
type recordingSpeechStreamer struct {
	recordingSpeechGenerator
}

func (r *recordingSpeechStreamer) StreamSpeech(
	_ context.Context,
	req provider.SpeechRequest,
) (io.ReadCloser, error) {
	r.calls++
	r.seen = append(r.seen, req.Auth)
	return io.NopCloser(strings.NewReader("streamed-audio")), nil
}

func TestSpeechRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request provider.SpeechRequest
		wantErr string
	}{
		{
			name:    "text",
			request: provider.SpeechRequest{Text: "hello there"},
		},
		{
			name: "every optional field set",
			request: provider.SpeechRequest{
				Model:        "eleven_flash_v2_5",
				Text:         "hello there",
				Voice:        "voice-1",
				OutputFormat: "pcm_16000",
				VoiceSetting: provider.VoiceSetting{Stability: 0.5, Speed: 1.1},
			},
		},
		{
			name:    "text is required",
			request: provider.SpeechRequest{},
			wantErr: "speech text is required",
		},
		{
			name:    "blank text is not text",
			request: provider.SpeechRequest{Text: "  \n "},
			wantErr: "speech text is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.request.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSpeechDecoratorResolvesOnEveryCall(t *testing.T) {
	recorder := &recordingSpeechGenerator{}
	call := 0
	generator := provider.WithSpeechDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			call++
			return core.Auth{Bearer: []string{"token-1", "token-2"}[call-1]}, nil
		},
	)

	for range 2 {
		_, err := generator.GenerateSpeech(
			context.Background(),
			provider.SpeechRequest{Text: "say hi"},
		)
		require.NoError(t, err)
	}
	require.Len(t, recorder.seen, 2)
	assert.Equal(t, "token-1", recorder.seen[0].Bearer)
	assert.Equal(t, "token-2", recorder.seen[1].Bearer)
}

func TestExplicitSpeechAuthOutranksDecorator(t *testing.T) {
	recorder := &recordingSpeechGenerator{}
	generator := provider.WithSpeechDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{
				Bearer:  "ambient",
				Headers: map[string]string{"X-Ambient": "yes"},
			}, nil
		},
	)

	_, err := generator.GenerateSpeech(context.Background(), provider.SpeechRequest{
		Text: "say hi",
		Auth: core.Auth{Bearer: "explicit"},
	})
	require.NoError(t, err)
	require.Len(t, recorder.seen, 1)
	assert.Equal(t, "explicit", recorder.seen[0].Bearer)
	assert.Equal(t, "yes", recorder.seen[0].Headers["X-Ambient"],
		"the decorator still contributes what the caller left unset")
}

func TestSpeechDecoratorErrorAbortsRequest(t *testing.T) {
	recorder := &recordingSpeechGenerator{}
	want := errors.New("credential store unavailable")
	generator := provider.WithSpeechDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{}, want
		},
	)

	_, err := generator.GenerateSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "say hi"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Zero(t, recorder.calls)
}

func TestNilSpeechDecoratorReturnsTheGeneratorUnchanged(t *testing.T) {
	recorder := &recordingSpeechGenerator{}
	assert.Same(t,
		provider.SpeechGenerator(recorder),
		provider.WithSpeechDecorator("recording", recorder, nil))
}

func TestSpeechDecoratorPreservesTheStreamingCapability(t *testing.T) {
	// Callers pick between blocking and streaming with a type assertion, so
	// wrapping must neither drop the streaming endpoint nor invent one.
	streaming := provider.WithSpeechDecorator(
		"recording",
		&recordingSpeechStreamer{},
		func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "token"}, nil
		},
	)
	streamer, ok := streaming.(provider.SpeechStreamer)
	require.True(t, ok, "a decorated streamer must still advertise SpeechStreamer")

	body, err := streamer.StreamSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "say hi"},
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, body.Close()) })
	audio, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "streamed-audio", string(audio))

	blocking := provider.WithSpeechDecorator(
		"recording",
		&recordingSpeechGenerator{},
		func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "token"}, nil
		},
	)
	_, ok = blocking.(provider.SpeechStreamer)
	assert.False(t, ok, "wrapping must not make a non-streamer claim SpeechStreamer")
}

func TestSpeechStreamDecoratorResolvesCredentials(t *testing.T) {
	recorder := &recordingSpeechStreamer{}
	generator := provider.WithSpeechDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "ambient"}, nil
		},
	)
	streamer, ok := generator.(provider.SpeechStreamer)
	require.True(t, ok)

	body, err := streamer.StreamSpeech(context.Background(), provider.SpeechRequest{
		Text: "say hi",
		Auth: core.Auth{Headers: map[string]string{"X-Trace": "abc"}},
	})
	require.NoError(t, err)
	require.NoError(t, body.Close())

	require.Len(t, recorder.seen, 1)
	assert.Equal(t, "ambient", recorder.seen[0].Bearer)
	assert.Equal(t, "abc", recorder.seen[0].Headers["X-Trace"])
}

func TestSpeechStreamDecoratorErrorAbortsRequest(t *testing.T) {
	recorder := &recordingSpeechStreamer{}
	want := errors.New("credential store unavailable")
	generator := provider.WithSpeechDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{}, want
		},
	)
	streamer, ok := generator.(provider.SpeechStreamer)
	require.True(t, ok)

	body, err := streamer.StreamSpeech(
		context.Background(),
		provider.SpeechRequest{Text: "say hi"},
	)
	require.Error(t, err)
	assert.Nil(t, body)
	assert.ErrorIs(t, err, want)
	assert.Zero(t, recorder.calls)
}

func TestNewSpeechRejectsUnknownProvider(t *testing.T) {
	_, err := provider.NewSpeech("bogus", provider.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
	assert.Contains(t, err.Error(), "elevenlabs", "the error should list what IS registered")
}

func TestNewSpeechRejectsUnsupportedCapabilityBeforeCredentialResolution(t *testing.T) {
	_, err := provider.NewSpeech("anthropic", provider.Options{
		LookupEnv: func(string) string { return "" },
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrUnsupportedCapability)

	var unsupported *provider.UnsupportedCapabilityError
	require.True(t, errors.As(err, &unsupported))
	assert.Equal(t, "anthropic", unsupported.Provider)
	assert.Equal(t, provider.CAPABILITY_SPEECH, unsupported.Capability)
}

func TestNewSpeechAllowsDeferredCredentialConstruction(t *testing.T) {
	for _, name := range []string{"elevenlabs", "minimax"} {
		t.Run(name, func(t *testing.T) {
			generator, err := provider.NewSpeech(name, provider.Options{
				LookupEnv: func(string) string { return "" },
				Decorator: func(context.Context) (core.Auth, error) {
					return core.Auth{Bearer: "resolved-per-request"}, nil
				},
			})
			require.NoError(t, err)
			assert.NotNil(t, generator)
		})
	}
}

func TestNewSpeechKeepsAdapterStreamingCapability(t *testing.T) {
	// ElevenLabs streams and MiniMax does not; the registry path must report
	// each honestly, decorator or no decorator.
	streaming, err := provider.NewSpeech("elevenlabs", provider.Options{
		APIKey:    "test-key",
		LookupEnv: func(string) string { return "" },
	})
	require.NoError(t, err)
	_, ok := streaming.(provider.SpeechStreamer)
	assert.True(t, ok, "elevenlabs exposes a streaming endpoint")

	blocking, err := provider.NewSpeech("minimax", provider.Options{
		LookupEnv: func(string) string { return "" },
		Decorator: func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "resolved-per-request"}, nil
		},
	})
	require.NoError(t, err)
	_, ok = blocking.(provider.SpeechStreamer)
	assert.False(t, ok, "minimax has no streaming speech endpoint in this milestone")
}
