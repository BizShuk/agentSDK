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

type recordingMusicGenerator struct {
	seen  []core.Auth
	calls int
}

func (r *recordingMusicGenerator) GenerateMusic(
	_ context.Context,
	req provider.MusicRequest,
) (provider.MusicResult, error) {
	r.calls++
	r.seen = append(r.seen, req.Auth)
	return provider.MusicResult{
		Audio: provider.MusicAsset{URL: "https://example.test/music.mp3"},
	}, nil
}

func TestMusicRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request provider.MusicRequest
		wantErr string
	}{
		{
			name:    "prompt",
			request: provider.MusicRequest{Prompt: "late night jazz"},
		},
		{
			name: "cover URL",
			request: provider.MusicRequest{
				Prompt:   "late night jazz",
				AudioURL: "https://example.test/original.mp3",
			},
		},
		{
			name:    "input required",
			request: provider.MusicRequest{},
			wantErr: "music input is required",
		},
		{
			name: "cover sources are mutually exclusive",
			request: provider.MusicRequest{
				Prompt:      "late night jazz",
				AudioURL:    "https://example.test/original.mp3",
				AudioBase64: "bXVzaWM=",
			},
			wantErr: "music cover sources are mutually exclusive",
		},
		{
			name: "sample rate cannot be negative",
			request: provider.MusicRequest{
				Prompt: "late night jazz",
				AudioSetting: provider.MusicAudioSetting{
					SampleRate: -1,
				},
			},
			wantErr: "sample rate must not be negative",
		},
		{
			name: "bitrate cannot be negative",
			request: provider.MusicRequest{
				Prompt: "late night jazz",
				AudioSetting: provider.MusicAudioSetting{
					Bitrate: -1,
				},
			},
			wantErr: "bitrate must not be negative",
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

func TestMusicDecoratorResolvesOnEveryCall(t *testing.T) {
	recorder := &recordingMusicGenerator{}
	call := 0
	generator := provider.WithMusicDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			call++
			return core.Auth{Bearer: []string{"token-1", "token-2"}[call-1]}, nil
		},
	)

	for range 2 {
		_, err := generator.GenerateMusic(
			context.Background(),
			provider.MusicRequest{Prompt: "compose"},
		)
		require.NoError(t, err)
	}
	require.Len(t, recorder.seen, 2)
	assert.Equal(t, "token-1", recorder.seen[0].Bearer)
	assert.Equal(t, "token-2", recorder.seen[1].Bearer)
}

func TestExplicitMusicAuthOutranksDecorator(t *testing.T) {
	recorder := &recordingMusicGenerator{}
	generator := provider.WithMusicDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{
				Bearer:  "ambient",
				Headers: map[string]string{"X-Ambient": "yes"},
			}, nil
		},
	)

	_, err := generator.GenerateMusic(context.Background(), provider.MusicRequest{
		Prompt: "compose",
		Auth:   core.Auth{Bearer: "explicit"},
	})
	require.NoError(t, err)
	require.Len(t, recorder.seen, 1)
	assert.Equal(t, "explicit", recorder.seen[0].Bearer)
	assert.Equal(t, "yes", recorder.seen[0].Headers["X-Ambient"])
}

func TestMusicDecoratorErrorAbortsRequest(t *testing.T) {
	recorder := &recordingMusicGenerator{}
	want := errors.New("credential store unavailable")
	generator := provider.WithMusicDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{}, want
		},
	)

	_, err := generator.GenerateMusic(
		context.Background(),
		provider.MusicRequest{Prompt: "compose"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Zero(t, recorder.calls)
}
