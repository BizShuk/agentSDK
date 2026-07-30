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

type recordingVideoGenerator struct {
	seen  []core.Auth
	calls int
}

func (r *recordingVideoGenerator) MaxPromptLength() int {
	return 3000
}

func (r *recordingVideoGenerator) GenerateVideo(
	_ context.Context,
	req provider.VideoRequest,
) (provider.VideoResult, error) {
	r.calls++
	r.seen = append(r.seen, req.Auth)
	return provider.VideoResult{Path: req.OutputPath}, nil
}

func TestVideoRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request provider.VideoRequest
		wantErr string
	}{
		{
			name: "text",
			request: provider.VideoRequest{
				Mode:       provider.VIDEO_MODE_TEXT,
				Prompt:     "a calm ocean",
				OutputPath: "ocean.mp4",
			},
		},
		{
			name: "image",
			request: provider.VideoRequest{
				Mode:          provider.VIDEO_MODE_IMAGE,
				Prompt:        "the clouds move",
				FirstFrameURL: "https://example.test/first.png",
				OutputPath:    "clouds.mp4",
			},
		},
		{
			name: "start and end",
			request: provider.VideoRequest{
				Mode:          provider.VIDEO_MODE_START_END,
				Prompt:        "grow up",
				FirstFrameURL: "https://example.test/young.png",
				LastFrameURL:  "https://example.test/old.png",
				OutputPath:    "aged.mp4",
			},
		},
		{
			name: "subject",
			request: provider.VideoRequest{
				Mode:             provider.VIDEO_MODE_SUBJECT,
				Prompt:           "walk through the city",
				SubjectImageURLs: []string{"https://example.test/person.png"},
				OutputPath:       "city.mp4",
			},
		},
		{
			name: "mode required",
			request: provider.VideoRequest{
				Prompt:     "scene",
				OutputPath: "scene.mp4",
			},
			wantErr: "mode is required",
		},
		{
			name: "unknown mode",
			request: provider.VideoRequest{
				Mode:       "unknown",
				Prompt:     "scene",
				OutputPath: "scene.mp4",
			},
			wantErr: "unsupported video mode",
		},
		{
			name: "prompt required",
			request: provider.VideoRequest{
				Mode:       provider.VIDEO_MODE_TEXT,
				OutputPath: "scene.mp4",
			},
			wantErr: "prompt is required",
		},
		{
			name: "output path required",
			request: provider.VideoRequest{
				Mode:   provider.VIDEO_MODE_TEXT,
				Prompt: "scene",
			},
			wantErr: "output path is required",
		},
		{
			name: "duration cannot be negative",
			request: provider.VideoRequest{
				Mode:       provider.VIDEO_MODE_TEXT,
				Prompt:     "scene",
				OutputPath: "scene.mp4",
				Duration:   -1,
			},
			wantErr: "duration must not be negative",
		},
		{
			name: "image requires first frame",
			request: provider.VideoRequest{
				Mode:       provider.VIDEO_MODE_IMAGE,
				Prompt:     "scene",
				OutputPath: "scene.mp4",
			},
			wantErr: "first frame URL is required",
		},
		{
			name: "start and end requires last frame",
			request: provider.VideoRequest{
				Mode:          provider.VIDEO_MODE_START_END,
				Prompt:        "scene",
				FirstFrameURL: "https://example.test/first.png",
				OutputPath:    "scene.mp4",
			},
			wantErr: "last frame URL is required",
		},
		{
			name: "subject requires images",
			request: provider.VideoRequest{
				Mode:       provider.VIDEO_MODE_SUBJECT,
				Prompt:     "scene",
				OutputPath: "scene.mp4",
			},
			wantErr: "subject image URL is required",
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

func TestVideoDecoratorResolvesOnEveryCall(t *testing.T) {
	recorder := &recordingVideoGenerator{}
	call := 0
	generator := provider.WithVideoDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			call++
			return core.Auth{Bearer: []string{"token-1", "token-2"}[call-1]}, nil
		},
	)

	for range 2 {
		_, err := generator.GenerateVideo(context.Background(), provider.VideoRequest{
			Mode:       provider.VIDEO_MODE_TEXT,
			Prompt:     "animate",
			OutputPath: "out.mp4",
		})
		require.NoError(t, err)
	}
	require.Len(t, recorder.seen, 2)
	assert.Equal(t, "token-1", recorder.seen[0].Bearer)
	assert.Equal(t, "token-2", recorder.seen[1].Bearer)
}

func TestExplicitVideoAuthOutranksDecorator(t *testing.T) {
	recorder := &recordingVideoGenerator{}
	generator := provider.WithVideoDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{
				Bearer:  "ambient",
				Headers: map[string]string{"X-Ambient": "yes"},
			}, nil
		},
	)

	_, err := generator.GenerateVideo(context.Background(), provider.VideoRequest{
		Mode:       provider.VIDEO_MODE_TEXT,
		Prompt:     "animate",
		OutputPath: "out.mp4",
		Auth:       core.Auth{Bearer: "explicit"},
	})
	require.NoError(t, err)
	require.Len(t, recorder.seen, 1)
	assert.Equal(t, "explicit", recorder.seen[0].Bearer)
	assert.Equal(t, "yes", recorder.seen[0].Headers["X-Ambient"])
}

func TestVideoDecoratorErrorAbortsRequest(t *testing.T) {
	recorder := &recordingVideoGenerator{}
	want := errors.New("credential store unavailable")
	generator := provider.WithVideoDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{}, want
		},
	)

	_, err := generator.GenerateVideo(context.Background(), provider.VideoRequest{
		Mode:       provider.VIDEO_MODE_TEXT,
		Prompt:     "animate",
		OutputPath: "out.mp4",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Zero(t, recorder.calls)
}
