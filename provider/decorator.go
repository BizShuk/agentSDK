package provider

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
)

// Decorator resolves the credential for one outbound request.
//
// It is a function of ctx rather than a value because OAuth access tokens
// expire mid-run. A credential resolved once at construction time is a
// header that stops working an hour in, and the failure looks like the
// provider rejecting a perfectly good agent. Resolving per request also
// covers the paths a construction-time credential silently misses: retry
// after a 401, an SSE stream reconnecting, ListModels, CountTokens.
//
// The type is defined HERE and not in the auth module on purpose. If the
// seam belonged to auth, every adapter that accepts one would import auth,
// and "an adapter compiles and tests with no credential machinery in
// scope" would stop being true — which is precisely the property that led
// four adapters to hand-roll their own OAuth rather than depend on it.
// provider/credential supplies the implementations; adapters never see it.
type Decorator func(ctx context.Context) (core.Auth, error)

// decorated wraps an Adapter so every call resolves the credential first
// and hands it down through core.ModelRequest.Auth.
//
// Delegating through the existing per-request Auth override — rather than
// rebuilding the adapter on token rotation, which is what the older
// RefreshingProvider did — means rotation costs one struct copy instead of
// a new HTTP client, and an in-flight stream is unaffected.
type decorated struct {
	Adapter
	decorate Decorator
}

// Generate implements core.Provider.
func (d *decorated) Generate(ctx context.Context, req core.ModelRequest) (core.ModelResult, error) {
	req, err := d.apply(ctx, req)
	if err != nil {
		return core.ModelResult{}, err
	}
	return d.Adapter.Generate(ctx, req)
}

// Stream implements core.Provider.
func (d *decorated) Stream(ctx context.Context, req core.ModelRequest) (<-chan core.ModelChunk, error) {
	req, err := d.apply(ctx, req)
	if err != nil {
		return nil, err
	}
	return d.Adapter.Stream(ctx, req)
}

// apply resolves the credential and merges it UNDER whatever the caller
// already put on the request. An explicit per-call Auth is the caller
// speaking directly about this one request and outranks the ambient
// credential; the decorator only fills what the caller left blank.
func (d *decorated) apply(ctx context.Context, req core.ModelRequest) (core.ModelRequest, error) {
	resolved, err := d.decorate(ctx)
	if err != nil {
		return req, fmt.Errorf("provider %s: resolve credential: %w", d.Adapter.ID(), err)
	}
	req.Auth = resolved.Merge(req.Auth)
	return req, nil
}

// WithDecorator returns a wrapped Adapter that resolves its credential
// before every call. A nil Decorator returns the adapter unchanged, so a
// caller can pass one through unconditionally.
//
// ListModels is deliberately NOT forwarded here. Promoting it would make
// every decorated adapter advertise core.ModelLister even when the one it
// wraps cannot list, and callers type-assert on that interface to decide
// whether to query live or fall back to the bundled catalog.
func WithDecorator(a Adapter, d Decorator) Adapter {
	if d == nil || a == nil {
		return a
	}
	return &decorated{Adapter: a, decorate: d}
}
