package credential_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	_ "github.com/bizshuk/agentsdk/provider/all"
	"github.com/bizshuk/agentsdk/provider/credential"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteIDTranslatesTheNamesThatDiffer(t *testing.T) {
	// The two vocabularies do not agree on spelling. If this table ever
	// collapses into an identity mapping, these two rows are why it
	// cannot.
	cases := []struct{ name, kind, want string }{
		{"anthropic", core.CREDENTIAL_KIND_APIKEY, "anthropic"},
		{"anthropic", core.CREDENTIAL_KIND_OAUTH, "anthropic_oauth"},
		{"codex", core.CREDENTIAL_KIND_OAUTH, "openai_oauth"},
		{"grok", core.CREDENTIAL_KIND_OAUTH, "xai_oauth"},
		{"antigravity", core.CREDENTIAL_KIND_OAUTH, "antigravity_oauth"},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/"+tc.kind, func(t *testing.T) {
			got, err := credential.RouteID(tc.name, tc.kind)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRouteIDRejectsAuto(t *testing.T) {
	// "auto" is a question for the resolver, not a row in a table.
	_, err := credential.RouteID("anthropic", core.CREDENTIAL_KIND_AUTO)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto")
}

func TestRouteIDRejectsAnUnsupportedPair(t *testing.T) {
	// minimax has no stored-credential path at all; antigravity has no
	// API key path. Both must say so rather than silently fall back.
	_, err := credential.RouteID("minimax", core.CREDENTIAL_KIND_OAUTH)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "none")

	_, err = credential.RouteID("antigravity", core.CREDENTIAL_KIND_APIKEY)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oauth")
}

// TestEveryRoutedNameIsARegisteredProvider is the drift guard: a route
// naming an adapter this repository does not ship would be dead config
// that only fails when someone tries to use it.
func TestEveryRoutedNameIsARegisteredProvider(t *testing.T) {
	registered := map[string]bool{}
	for _, n := range provider.Names() {
		registered[n] = true
	}
	for _, name := range credential.Names() {
		assert.Truef(t, registered[name],
			"credential route names %q but no such adapter is registered", name)
	}
}

// TestRoutedKindsAreAdvertisedByTheAdapter keeps the table honest: a
// route promising a credential class the adapter refuses would resolve a
// credential and then fail at the first request, which is the worst place
// to learn about it.
//
// The check is against AuthSchemes, NOT against Metadata.OAuthEnv /
// APIKeyEnv. Those describe how the registry resolves a credential from
// the ENVIRONMENT; a route describes how credential resolves one from a
// STORE. They are orthogonal, and conflating them reads antigravity —
// which accepts an API key from the environment but only OAuth from auth
// — as a contradiction when it is simply both.
func TestRoutedKindsAreAdvertisedByTheAdapter(t *testing.T) {
	for _, name := range credential.Names() {
		entry, ok := provider.Lookup(name)
		require.Truef(t, ok, "provider %q must be registered", name)

		// A dummy key: we are asking what the adapter ACCEPTS, which is
		// static, not whether this credential works.
		p, err := entry.New(provider.Options{APIKey: "dummy"})
		require.NoErrorf(t, err, "provider %q must construct for capability inspection", name)

		advertised := map[string]bool{}
		for _, s := range p.AuthSchemes() {
			advertised[s] = true
		}
		for _, kind := range credential.Kinds(name) {
			assert.Truef(t, advertised[kind],
				"credential routes %s/%s but the adapter advertises only %v",
				name, kind, p.AuthSchemes())
		}
	}
}
