package credential

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/auth/model"
	authprovider "github.com/bizshuk/auth/provider"
	svc "github.com/bizshuk/auth/svc"
)

// The OAuth flow itself — PKCE, the authorize URL, the code exchange, the
// local callback server, opening a browser, refreshing — lives in the auth
// module and is NOT reimplemented here.
//
// It used to be reimplemented, four times: provider/{anthropic,codex,grok,
// antigravity}/auth_oauth.go each carried its own copy, ~240 lines apiece,
// with a comment explaining that the duplication kept the adapter free of
// a cross-module import. The comment was right about the goal and wrong
// about the method: the goal is met by keeping auth behind THIS package,
// which the adapters do not import.
//
// The copies had already drifted from the auth module by the time they
// were removed — different token endpoints, different redirect URIs,
// different scopes — with no caller on either side to say which was
// correct. That is the failure mode this file exists to prevent.

// Store is the credential store a Source reads from. It is satisfied by
// auth/utils.FileStore; the interface exists so a caller can substitute an
// in-memory or remote store without this package growing an option.
type Store = svc.ResolverStore

// Source turns stored credentials into a provider.Decorator.
//
// One Source serves one adapter. It holds the resolver rather than a
// resolved credential precisely so that each call re-resolves: that is
// what refreshes an expired OAuth token and picks up a rotation.
type Source struct {
	resolver *svc.Resolver
	name     string // agentsdk registry name, for error messages
	family   string // auth provider family the resolver keys on
}

// NewSource builds a Source for one agentsdk provider name and credential
// kind. The kind must be explicit (api_key / oauth): "auto" precedence is
// resolved by the store and environment at call time, not by naming a
// route up front, so pass core.CREDENTIAL_KIND_AUTO to NewAutoSource.
func NewSource(store Store, name, kind string) (*Source, error) {
	id, err := RouteID(name, kind)
	if err != nil {
		return nil, err
	}
	return newSource(store, name, id)
}

// NewAutoSource builds a Source that lets the resolver apply its own
// precedence — the active credential for the family, then any sibling,
// then the environment fallback. Use it when the config says nothing
// about which credential class to prefer.
func NewAutoSource(store Store, name string) (*Source, error) {
	kinds := Kinds(name)
	if len(kinds) == 0 {
		return nil, fmt.Errorf("credential: provider %q has no auth route; it can only read the environment", name)
	}
	// Any route for this name resolves to the same auth family; the
	// resolver keys on family, not on credential kind.
	id := routes[route{name, kinds[0]}]
	return newSource(store, name, id)
}

func newSource(store Store, name, routeID string) (*Source, error) {
	if store == nil {
		return nil, fmt.Errorf("credential: provider %q: store is required", name)
	}
	auth, err := authprovider.New(routeID)
	if err != nil {
		return nil, fmt.Errorf("credential: provider %q: %w", name, err)
	}
	resolver := svc.NewResolver(store,
		func(*model.Credential) (model.Authenticator, error) { return auth, nil },
		nil)
	return &Source{resolver: resolver, name: name, family: auth.Provider()}, nil
}

// Decorator returns the provider.Decorator this Source backs.
//
// Wire it through provider.Options so the registry attaches it during
// construction:
//
//	src, _ := credential.NewSource(store, "anthropic", core.CREDENTIAL_KIND_OAUTH)
//	p, _ := provider.New("anthropic", provider.Options{Decorator: src.Decorator()})
func (s *Source) Decorator() provider.Decorator {
	return func(ctx context.Context) (core.Auth, error) {
		cred, err := s.resolver.Resolve(ctx, s.family)
		if err != nil {
			return core.Auth{}, fmt.Errorf("credential: provider %q: %w", s.name, err)
		}
		return toAuth(cred), nil
	}
}

// toAuth projects an auth credential onto the wire-level shape adapters
// consume. The projection is deliberately narrow: an adapter is handed a
// token and an endpoint, never a refresh token, an expiry, or an account
// identity it has no business persisting.
func toAuth(cred *model.Credential) core.Auth {
	if cred == nil {
		return core.Auth{}
	}
	a := core.Auth{
		APIKey:  cred.APIKey,
		Bearer:  cred.AccessToken,
		BaseURL: cred.BaseURL,
	}
	// Codex identifies the paying account with a header rather than with
	// the token, so it has to ride along or the request 401s on an
	// otherwise valid credential.
	if cred.AccountID != "" {
		a.Headers = map[string]string{"ChatGPT-Account-ID": cred.AccountID}
	}
	return a
}

// Login runs the interactive credential flow for one provider name and
// kind, returning the credential the caller should persist.
//
// It is a thin pass-through to auth so that a CLI in this repository can
// speak agentsdk's two-axis vocabulary (name + kind) without importing
// auth or learning its flattened ids.
func Login(ctx context.Context, name, kind string, opts ...model.Option) (*model.Credential, error) {
	id, err := RouteID(name, kind)
	if err != nil {
		return nil, err
	}
	cred, err := authprovider.Login(ctx, id, opts...)
	if err != nil {
		return nil, fmt.Errorf("credential: login %s/%s: %w", name, kind, err)
	}
	return cred, nil
}
