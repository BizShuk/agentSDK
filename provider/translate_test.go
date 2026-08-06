package provider_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

func TestTranslateRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request provider.TranslateRequest
		wantErr string
	}{
		{
			name: "valid",
			request: provider.TranslateRequest{
				Text:           "hello",
				TargetLanguage: "es",
			},
		},
		{
			name:    "text required",
			request: provider.TranslateRequest{TargetLanguage: "es"},
			wantErr: "text is required",
		},
		{
			name:    "target language required",
			request: provider.TranslateRequest{Text: "hello"},
			wantErr: "target language is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

type fakeTranslator struct {
	got provider.TranslateRequest
}

func (f *fakeTranslator) Translate(
	_ context.Context,
	req provider.TranslateRequest,
) (provider.TranslateResult, error) {
	f.got = req
	return provider.TranslateResult{Text: "hola"}, nil
}

type fakeStreamingTranslator struct {
	fakeTranslator
}

func (f *fakeStreamingTranslator) StreamTranslation(
	_ context.Context,
	req provider.TranslateRequest,
) (<-chan provider.TranslateChunk, error) {
	f.got = req
	out := make(chan provider.TranslateChunk, 1)
	out <- provider.TranslateChunk{Text: "hola"}
	close(out)
	return out, nil
}

func TestWithTranslateDecoratorResolvesAuth(t *testing.T) {
	fake := &fakeTranslator{}
	decorated := provider.WithTranslateDecorator("google", fake,
		func(context.Context) (core.Auth, error) {
			return core.Auth{APIKey: "decorated-key"}, nil
		})

	result, err := decorated.Translate(context.Background(), provider.TranslateRequest{
		Text:           "hello",
		TargetLanguage: "es",
	})
	require.NoError(t, err)
	assert.Equal(t, "hola", result.Text)
	assert.Equal(t, "decorated-key", fake.got.Auth.APIKey)
}

func TestWithTranslateDecoratorPreservesStreamer(t *testing.T) {
	streaming := &fakeStreamingTranslator{}
	decorated := provider.WithTranslateDecorator("google", streaming,
		func(context.Context) (core.Auth, error) {
			return core.Auth{APIKey: "decorated-key"}, nil
		})

	streamer, ok := decorated.(provider.TranslateStreamer)
	require.True(t, ok, "decorated streaming translator must keep TranslateStreamer")
	chunks, err := streamer.StreamTranslation(context.Background(), provider.TranslateRequest{
		Text:           "hello",
		TargetLanguage: "es",
	})
	require.NoError(t, err)
	assert.Equal(t, "hola", (<-chunks).Text)
	assert.Equal(t, "decorated-key", streaming.got.Auth.APIKey)

	// A blocking-only translator must NOT gain the streaming surface.
	blocking := provider.WithTranslateDecorator("google", &fakeTranslator{},
		func(context.Context) (core.Auth, error) { return core.Auth{}, nil })
	_, streams := blocking.(provider.TranslateStreamer)
	assert.False(t, streams)
}

func TestTranslateCapabilityDiscovery(t *testing.T) {
	entry := provider.Entry{
		Name: "translate-only",
		NewTranslate: func(provider.ResolvedConfig) (provider.Translator, error) {
			return nil, nil
		},
	}
	assert.True(t, entry.Supports(provider.CAPABILITY_TRANSLATE))
	assert.False(t, entry.Supports(provider.CAPABILITY_LIVE))
	assert.Equal(t, []provider.Capability{provider.CAPABILITY_TRANSLATE}, entry.Capabilities())
}
