package config

import (
	"context"
	"fmt"
	"sync"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/auth/model"
	svc "github.com/bizshuk/auth/svc"
)

// BuildProvider constructs a concrete Provider from a resolved
// credential — typically a closure over provider/*.New with the credential's
// token, e.g. anthropic.New(anthropic.WithAPIKey(cred.APIKey)).
type BuildProvider func(cred *model.Credential) (core.Provider, error)

// RefreshingProvider wraps a Provider so every call resolves the
// provider-family credential first: an expired OAuth token is refreshed and
// persisted by svc.Resolver, and a rotated token rebuilds the inner
// provider before the call proceeds.
//
// It lives in config (not runtime) for the same reason DefaultMiddleware
// does: it wires a concrete mechanism (auth) onto a core port, and the SDK
// core packages must not depend on auth.
type RefreshingProvider struct {
	resolver *svc.Resolver
	family   string
	build    BuildProvider

	mu    sync.Mutex
	inner core.Provider
	token string
}

// NewRefreshingProvider decorates the provider family's credential flow onto
// a lazily-built Provider. The inner provider is first built on the
// first call, so construction itself never touches credentials.
func NewRefreshingProvider(resolver *svc.Resolver, family string, build BuildProvider) (*RefreshingProvider, error) {
	if resolver == nil {
		return nil, fmt.Errorf("refreshing provider: resolver is required")
	}
	if family == "" {
		return nil, fmt.Errorf("refreshing provider: provider family is required")
	}
	if build == nil {
		return nil, fmt.Errorf("refreshing provider: build func is required")
	}
	return &RefreshingProvider{resolver: resolver, family: family, build: build}, nil
}

// ID reports the provider family; it must not trigger credential I/O.
func (p *RefreshingProvider) ID() string { return p.family }

// Name is a backward-compat alias for ID. Returns the provider family.
// Deprecated: use ID.
func (p *RefreshingProvider) Name() string { return p.family }

// Models returns no models (the RefreshingProvider delegates to the inner
// provider, but this method exists to satisfy core.Provider without a
// credential lookup). Callers that need the catalog should reach the inner
// provider via build() during a separate, non-blocking path.
func (p *RefreshingProvider) Models() []core.ModelSpec { return nil }

// AuthSchemes is unknown without an inner provider; return both possible
// flavors so the runtime can pass auth through and let the inner decide.
func (p *RefreshingProvider) AuthSchemes() []string { return []string{"api_key", "oauth"} }

// Generate resolves (and if needed refreshes) the credential, then delegates.
func (p *RefreshingProvider) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	inner, err := p.current(ctx)
	if err != nil {
		return core.ModelResult{}, err
	}
	return inner.Generate(ctx, req)
}

// Stream resolves (and if needed refreshes) the credential, then delegates.
func (p *RefreshingProvider) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	inner, err := p.current(ctx)
	if err != nil {
		return nil, err
	}
	return inner.Stream(ctx, req)
}

// CountTokens resolves (and if needed refreshes) the credential, then delegates.
func (p *RefreshingProvider) CountTokens(ctx context.Context, msgs []core.Message) (int, error) {
	inner, err := p.current(ctx)
	if err != nil {
		return 0, err
	}
	return inner.CountTokens(ctx, msgs)
}

// current returns an inner provider built from the freshest credential,
// rebuilding only when the effective token rotated.
func (p *RefreshingProvider) current(ctx context.Context) (core.Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	cred, err := p.resolver.Resolve(ctx, p.family)
	if err != nil {
		return nil, fmt.Errorf("refreshing provider %q: %w", p.family, err)
	}
	token := credentialToken(cred)
	if p.inner != nil && token == p.token {
		return p.inner, nil
	}
	inner, err := p.build(cred)
	if err != nil {
		return nil, fmt.Errorf("refreshing provider %q: build: %w", p.family, err)
	}
	if inner == nil {
		return nil, fmt.Errorf("refreshing provider %q: build returned nil provider", p.family)
	}
	p.inner = inner
	p.token = token
	return p.inner, nil
}

func credentialToken(cred *model.Credential) string {
	if cred.AccessToken != "" {
		return cred.AccessToken
	}
	return cred.APIKey
}
