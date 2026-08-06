package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingTranscriber struct {
	seen  []core.Auth
	calls int
}

func (r *recordingTranscriber) Transcribe(
	_ context.Context,
	req provider.TranscribeRequest,
) (provider.TranscribeResult, error) {
	r.calls++
	r.seen = append(r.seen, req.Auth)
	return provider.TranscribeResult{Text: "hello"}, nil
}

func TestTranscribeRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request provider.TranscribeRequest
		wantErr string
	}{
		{
			name: "bytes",
			request: provider.TranscribeRequest{
				Audio: provider.AudioSource{Bytes: []byte("audio"), Format: "mp3"},
			},
		},
		{
			name: "URL",
			request: provider.TranscribeRequest{
				Audio: provider.AudioSource{URL: "https://example.test/clip.mp3"},
			},
		},
		{
			name:    "audio is required",
			request: provider.TranscribeRequest{},
			wantErr: "audio bytes or URL is required",
		},
		{
			name: "format alone is not an audio source",
			request: provider.TranscribeRequest{
				Audio: provider.AudioSource{Format: "mp3"},
			},
			wantErr: "audio bytes or URL is required",
		},
		{
			name: "blank URL is not an audio source",
			request: provider.TranscribeRequest{
				Audio: provider.AudioSource{URL: "   "},
			},
			wantErr: "audio bytes or URL is required",
		},
		{
			name: "sources are mutually exclusive",
			request: provider.TranscribeRequest{
				Audio: provider.AudioSource{
					Bytes: []byte("audio"),
					URL:   "https://example.test/clip.mp3",
				},
			},
			wantErr: "mutually exclusive",
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

func TestTranscriberDecoratorResolvesOnEveryCall(t *testing.T) {
	recorder := &recordingTranscriber{}
	call := 0
	transcriber := provider.WithTranscriberDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			call++
			return core.Auth{Bearer: []string{"token-1", "token-2"}[call-1]}, nil
		},
	)

	for range 2 {
		_, err := transcriber.Transcribe(context.Background(), transcribeFixture())
		require.NoError(t, err)
	}
	require.Len(t, recorder.seen, 2)
	assert.Equal(t, "token-1", recorder.seen[0].Bearer)
	assert.Equal(t, "token-2", recorder.seen[1].Bearer)
}

func TestExplicitTranscribeAuthOutranksDecorator(t *testing.T) {
	recorder := &recordingTranscriber{}
	transcriber := provider.WithTranscriberDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{
				Bearer:  "ambient",
				Headers: map[string]string{"X-Ambient": "yes"},
			}, nil
		},
	)

	request := transcribeFixture()
	request.Auth = core.Auth{Bearer: "explicit"}
	_, err := transcriber.Transcribe(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, recorder.seen, 1)
	assert.Equal(t, "explicit", recorder.seen[0].Bearer)
	assert.Equal(t, "yes", recorder.seen[0].Headers["X-Ambient"],
		"the decorator still contributes what the caller left unset")
}

func TestTranscriberDecoratorErrorAbortsRequest(t *testing.T) {
	recorder := &recordingTranscriber{}
	want := errors.New("credential store unavailable")
	transcriber := provider.WithTranscriberDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{}, want
		},
	)

	_, err := transcriber.Transcribe(context.Background(), transcribeFixture())
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Zero(t, recorder.calls)
}

func TestNilTranscriberDecoratorReturnsTheTranscriberUnchanged(t *testing.T) {
	recorder := &recordingTranscriber{}
	assert.Same(t,
		provider.Transcriber(recorder),
		provider.WithTranscriberDecorator("recording", recorder, nil))
}

func TestNewTranscriberRejectsUnknownProvider(t *testing.T) {
	_, err := provider.NewTranscriber("bogus", provider.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
	assert.Contains(t, err.Error(), "elevenlabs", "the error should list what IS registered")
}

func TestNewTranscriberRejectsUnsupportedCapabilityBeforeCredentialResolution(t *testing.T) {
	_, err := provider.NewTranscriber("minimax", provider.Options{
		LookupEnv: func(string) string { return "" },
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrUnsupportedCapability)

	var unsupported *provider.UnsupportedCapabilityError
	require.True(t, errors.As(err, &unsupported))
	assert.Equal(t, "minimax", unsupported.Provider)
	assert.Equal(t, provider.CAPABILITY_TRANSCRIBE, unsupported.Capability)
}

func TestNewTranscriberAllowsDeferredCredentialConstruction(t *testing.T) {
	transcriber, err := provider.NewTranscriber("elevenlabs", provider.Options{
		LookupEnv: func(string) string { return "" },
		Decorator: func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "resolved-per-request"}, nil
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, transcriber)
}

func transcribeFixture() provider.TranscribeRequest {
	return provider.TranscribeRequest{
		Audio: provider.AudioSource{Bytes: []byte("audio"), Format: "mp3"},
	}
}
