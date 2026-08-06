package provider_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
)

func TestLiveRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request provider.LiveRequest
		wantErr string
	}{
		{
			name:    "empty request defaults to text modality",
			request: provider.LiveRequest{},
		},
		{
			name:    "text modality",
			request: provider.LiveRequest{ResponseModality: provider.LIVE_MODALITY_TEXT},
		},
		{
			name:    "audio modality",
			request: provider.LiveRequest{ResponseModality: provider.LIVE_MODALITY_AUDIO},
		},
		{
			name:    "unknown modality rejected",
			request: provider.LiveRequest{ResponseModality: "video"},
			wantErr: "response modality",
		},
		{
			name: "translation requires target language",
			request: provider.LiveRequest{
				Translation: &provider.LiveTranslation{TargetLanguage: "  "},
			},
			wantErr: "target language",
		},
		{
			name: "translation with target language",
			request: provider.LiveRequest{
				Translation: &provider.LiveTranslation{TargetLanguage: "es"},
			},
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

type fakeLiveConnector struct {
	got provider.LiveRequest
}

func (f *fakeLiveConnector) ConnectLive(
	_ context.Context,
	req provider.LiveRequest,
) (provider.LiveSession, error) {
	f.got = req
	return nil, nil
}

func TestWithLiveDecoratorResolvesAuthAtConnect(t *testing.T) {
	fake := &fakeLiveConnector{}
	decorated := provider.WithLiveDecorator("google", fake,
		func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "decorated-token"}, nil
		})

	_, err := decorated.ConnectLive(context.Background(), provider.LiveRequest{})
	require.NoError(t, err)
	assert.Equal(t, "decorated-token", fake.got.Auth.Bearer)
}

func TestWithLiveDecoratorExplicitAuthWins(t *testing.T) {
	fake := &fakeLiveConnector{}
	decorated := provider.WithLiveDecorator("google", fake,
		func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "decorated-token"}, nil
		})

	_, err := decorated.ConnectLive(context.Background(), provider.LiveRequest{
		Auth: core.Auth{Bearer: "explicit-token"},
	})
	require.NoError(t, err)
	assert.Equal(t, "explicit-token", fake.got.Auth.Bearer)
}

func TestWithLiveDecoratorNilPassthrough(t *testing.T) {
	fake := &fakeLiveConnector{}
	assert.Same(t, fake, provider.WithLiveDecorator("google", fake, nil).(*fakeLiveConnector))
}

func TestLiveCapabilityDiscovery(t *testing.T) {
	entry := provider.Entry{
		Name: "live-only",
		NewLive: func(provider.ResolvedConfig) (provider.LiveConnector, error) {
			return nil, nil
		},
	}
	assert.True(t, entry.Supports(provider.CAPABILITY_LIVE))
	assert.False(t, entry.Supports(provider.CAPABILITY_TRANSLATE))
	assert.Equal(t, []provider.Capability{provider.CAPABILITY_LIVE}, entry.Capabilities())
}
