package config_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/auth"
	"github.com/bizshuk/agentsdk/config"
	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingProvider struct {
	token string
	calls int
}

func (p *recordingProvider) Name() string { return "recording" }

func (p *recordingProvider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	p.calls++
	return core.ModelResult{Text: p.token, StopReason: "end_turn"}, nil
}

func (p *recordingProvider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	ch := make(chan core.ModelChunk)
	close(ch)
	return ch, nil
}

func (p *recordingProvider) CountTokens(ctx context.Context, msgs []core.Message) (int, error) {
	return 0, nil
}

type rotatingAuthenticator struct {
	serial int
}

func (a *rotatingAuthenticator) Provider() string { return "openai" }

func (a *rotatingAuthenticator) Kind() auth.Kind { return auth.KIND_OAUTH }

func (a *rotatingAuthenticator) Login(ctx context.Context) (*auth.Credential, error) {
	return nil, errors.New("login not supported")
}

func (a *rotatingAuthenticator) Verify(ctx context.Context, cred *auth.Credential) (*auth.VerifyResult, error) {
	return nil, errors.New("verify not supported")
}

func (a *rotatingAuthenticator) Refresh(ctx context.Context, cred *auth.Credential) (*auth.Credential, error) {
	a.serial++
	rotated := *cred
	rotated.AccessToken = "rotated-" + string(rune('0'+a.serial))
	rotated.ExpiresAt = time.Now().Add(time.Hour)
	return &rotated, nil
}

func newOAuthStore(t *testing.T, expiresAt time.Time) *auth.FileStore {
	t.Helper()
	store, err := auth.NewFileStore(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.Save(&auth.Credential{
		Provider: "openai", Kind: auth.KIND_OAUTH,
		AccessToken: "initial", RefreshToken: "refresh",
		ExpiresAt: expiresAt,
	}))
	return store
}

func TestRefreshingProviderRefreshesExpiredBeforeCall(t *testing.T) {
	store := newOAuthStore(t, time.Now().Add(-time.Minute))
	authenticator := &rotatingAuthenticator{}
	resolver := auth.NewResolver(store, func(*auth.Credential) (auth.Authenticator, error) {
		return authenticator, nil
	}, nil)

	builds := 0
	provider, err := config.NewRefreshingProvider(resolver, "openai", func(cred *auth.Credential) (core.ModelProvider, error) {
		builds++
		return &recordingProvider{token: cred.AccessToken}, nil
	})
	require.NoError(t, err)

	result, err := provider.Generate(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	assert.Equal(t, "rotated-1", result.Text, "call must use the refreshed token")
	assert.Equal(t, 1, builds)

	// The rotated credential was persisted, so the next call sees a fresh
	// token and must NOT refresh or rebuild again.
	result, err = provider.Generate(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	assert.Equal(t, "rotated-1", result.Text)
	assert.Equal(t, 1, builds, "unchanged token must reuse the inner provider")
	assert.Equal(t, 1, authenticator.serial, "valid token must not be refreshed again")
}

func TestRefreshingProviderRebuildsOnRotation(t *testing.T) {
	store := newOAuthStore(t, time.Now().Add(time.Hour))
	resolver := auth.NewResolver(store, nil, nil)

	builds := 0
	provider, err := config.NewRefreshingProvider(resolver, "openai", func(cred *auth.Credential) (core.ModelProvider, error) {
		builds++
		return &recordingProvider{token: cred.AccessToken}, nil
	})
	require.NoError(t, err)

	result, err := provider.Generate(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	assert.Equal(t, "initial", result.Text)

	// Out-of-band rotation (another process saved a new token): the next
	// call must pick it up and rebuild the inner provider.
	require.NoError(t, store.Save(&auth.Credential{
		Provider: "openai", Kind: auth.KIND_OAUTH,
		AccessToken: "out-of-band", RefreshToken: "refresh",
		ExpiresAt: time.Now().Add(time.Hour),
	}))
	result, err = provider.Generate(context.Background(), core.ModelRequest{})
	require.NoError(t, err)
	assert.Equal(t, "out-of-band", result.Text)
	assert.Equal(t, 2, builds)
}

func TestRefreshingProviderSurfacesResolveFailure(t *testing.T) {
	store, err := auth.NewFileStore(t.TempDir())
	require.NoError(t, err)
	resolver := auth.NewResolver(store, nil, nil)

	provider, err := config.NewRefreshingProvider(resolver, "openai", func(*auth.Credential) (core.ModelProvider, error) {
		t.Fatal("build must not run without a credential")
		return nil, nil
	})
	require.NoError(t, err)

	_, err = provider.Generate(context.Background(), core.ModelRequest{})
	var unavailableErr *auth.UnavailableError
	require.ErrorAs(t, err, &unavailableErr)
}

func TestNewRefreshingProviderValidatesArguments(t *testing.T) {
	store, err := auth.NewFileStore(t.TempDir())
	require.NoError(t, err)
	resolver := auth.NewResolver(store, nil, nil)
	build := func(*auth.Credential) (core.ModelProvider, error) { return &recordingProvider{}, nil }

	_, err = config.NewRefreshingProvider(nil, "openai", build)
	assert.ErrorContains(t, err, "resolver is required")
	_, err = config.NewRefreshingProvider(resolver, "", build)
	assert.ErrorContains(t, err, "provider family is required")
	_, err = config.NewRefreshingProvider(resolver, "openai", nil)
	assert.ErrorContains(t, err, "build func is required")
}
