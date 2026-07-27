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

type recordingImageGenerator struct {
	seen  []core.Auth
	calls int
}

func (r *recordingImageGenerator) GenerateImage(
	_ context.Context,
	req provider.ImageRequest,
) (provider.ImageResult, error) {
	r.calls++
	r.seen = append(r.seen, req.Auth)
	return provider.ImageResult{
		Images: []provider.Image{{Base64: "aW1hZ2U="}},
	}, nil
}

func TestImageDecoratorResolvesOnEveryCall(t *testing.T) {
	recorder := &recordingImageGenerator{}
	call := 0
	generator := provider.WithImageDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			call++
			return core.Auth{Bearer: []string{"token-1", "token-2"}[call-1]}, nil
		},
	)

	for range 2 {
		_, err := generator.GenerateImage(
			context.Background(),
			provider.ImageRequest{Prompt: "draw"},
		)
		require.NoError(t, err)
	}
	require.Len(t, recorder.seen, 2)
	assert.Equal(t, "token-1", recorder.seen[0].Bearer)
	assert.Equal(t, "token-2", recorder.seen[1].Bearer)
}

func TestExplicitImageAuthOutranksDecorator(t *testing.T) {
	recorder := &recordingImageGenerator{}
	generator := provider.WithImageDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{
				Bearer:  "ambient",
				Headers: map[string]string{"X-Ambient": "yes"},
			}, nil
		},
	)

	_, err := generator.GenerateImage(context.Background(), provider.ImageRequest{
		Prompt: "draw",
		Auth:   core.Auth{Bearer: "explicit"},
	})
	require.NoError(t, err)
	require.Len(t, recorder.seen, 1)
	assert.Equal(t, "explicit", recorder.seen[0].Bearer)
	assert.Equal(t, "yes", recorder.seen[0].Headers["X-Ambient"])
}

func TestImageDecoratorErrorAbortsRequest(t *testing.T) {
	recorder := &recordingImageGenerator{}
	want := errors.New("credential store unavailable")
	generator := provider.WithImageDecorator(
		"recording",
		recorder,
		func(context.Context) (core.Auth, error) {
			return core.Auth{}, want
		},
	)

	_, err := generator.GenerateImage(
		context.Background(),
		provider.ImageRequest{Prompt: "draw"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Zero(t, recorder.calls)
}
