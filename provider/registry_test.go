package provider_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamesAreSortedAndComplete(t *testing.T) {
	// The registered set depends on what the linking binary imports; the
	// test binary blank-imports provider/all (see providers_test.go), so
	// every built-in adapter must appear.
	got := provider.Names()
	require.NotEmpty(t, got)

	// Names() must be sorted for stable menu output — verify by checking
	// every adjacent pair is non-decreasing.
	for i := 1; i < len(got); i++ {
		assert.LessOrEqualf(t, got[i-1], got[i],
			"Names() must be sorted; got %v", got)
	}

	want := []string{"anthropic", "antigravity", "codex", "google", "grok", "minimax", "ollama"}
	for _, name := range want {
		assert.Containsf(t, got, name,
			"expected built-in provider %q to be registered", name)
	}
}

func TestCoreDefaultProviderIsRegistered(t *testing.T) {
	// provider.DEFAULT_NAME is the only source of truth for the default
	// provider name. If anyone renames it or stops registering the
	// adapter it points at, this test fails.
	_, ok := provider.Lookup(provider.DEFAULT_NAME)
	assert.Truef(t, ok, "provider.DEFAULT_NAME %q must be a registered provider", provider.DEFAULT_NAME)
}

func TestLookupNormalizesName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"minimax", "minimax"},
		{"MiniMax", "minimax"},
		{"  ANTHROPIC  ", "anthropic"},
		{"", provider.DEFAULT_NAME},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			e, ok := provider.Lookup(tc.in)
			require.True(t, ok)
			assert.Equal(t, tc.want, e.Name)
		})
	}
}

func TestLookupUnknown(t *testing.T) {
	_, ok := provider.Lookup("bogus")
	assert.False(t, ok)

	_, err := provider.New("bogus", provider.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
	assert.Contains(t, err.Error(), "minimax", "the error should list what IS registered")
}

func TestEveryEntryIsSelfDescribing(t *testing.T) {
	for _, e := range provider.Entries() {
		t.Run(e.Name, func(t *testing.T) {
			assert.NotEmpty(t, e.Name)
			assert.NotEmpty(t, e.Metadata.Label, "a wizard menu renders Label")
			// Each registered entry must document at least one credential
			// path: an OAuthEnv (looked up first in auto mode), an
			// APIKeyEnv, or a constructor-based path (e.g. codex's OAuth
			// is provided via NewWithOAuth, not an env var). Entries that
			// declare NONE of the above have no way to bootstrap a
			// credential through the registry at all.
			hasEnvPath := len(e.Metadata.OAuthEnv) > 0 || len(e.Metadata.APIKeyEnv) > 0
			hasConstructorPath := strings.Contains(strings.ToLower(e.Metadata.Note), "constructor")
			assert.Truef(t, hasEnvPath || hasConstructorPath,
				"entry %q has no credential path (OAuthEnv=%v APIKeyEnv=%v Note=%q); declare an env OR document a constructor path",
				e.Name, e.Metadata.OAuthEnv, e.Metadata.APIKeyEnv, e.Metadata.Note)
			assert.NotNil(t, e.New)
			assert.NotNil(t, e.Catalog, "every entry must expose its bundled catalog")
		})
	}
}

func TestCredentialResolutionPrecedence(t *testing.T) {
	// Explicit APIKey wins over the environment; otherwise the entry's
	// env var list is tried in order. Anthropic's ordering (OAuth token
	// before API key) is the reason the list is ordered at all.
	env := map[string]string{
		"ANTHROPIC_OAUTH_TOKEN": "from-oauth",
		"ANTHROPIC_API_KEY":     "from-api-key",
		"MINIMAX_API_KEY":       "mini-key",
		"MINIMAX_BASE_URL":      "https://example.test/v1",
	}
	lookup := func(k string) string { return env[k] }

	cases := []struct {
		name        string
		provider    string
		opts        provider.Options
		wantAPIKey  string
		wantBaseURL string
		wantErr     string
	}{
		{
			name: "oauth outranks api key", provider: "anthropic",
			opts:       provider.Options{LookupEnv: lookup},
			wantAPIKey: "from-oauth",
		},
		{
			name: "explicit key outranks env", provider: "anthropic",
			opts:       provider.Options{APIKey: "explicit", LookupEnv: lookup},
			wantAPIKey: "explicit",
		},
		{
			name: "base url from env", provider: "minimax",
			opts:        provider.Options{LookupEnv: lookup},
			wantAPIKey:  "mini-key",
			wantBaseURL: "https://example.test/v1",
		},
		{
			name: "APIKeyEnv override redirects the lookup", provider: "minimax",
			opts:       provider.Options{APIKeyEnv: "ANTHROPIC_API_KEY", LookupEnv: lookup},
			wantAPIKey: "from-api-key",
		},
		{
			name: "strict oauth picks OAuth env over API key", provider: "anthropic",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_OAUTH,
				LookupEnv:      lookup,
			},
			wantAPIKey: "from-oauth",
		},
		{
			name: "strict api_key ignores OAuth env", provider: "anthropic",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_APIKEY,
				LookupEnv:      lookup,
			},
			wantAPIKey: "from-api-key",
		},
		{
			name: "strict api_key rejects when no API key env set", provider: "anthropic",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_APIKEY,
				LookupEnv:      func(string) string { return "" },
			},
			wantErr: "requires api_key credential",
		},
		{
			name: "strict oauth rejects provider without OAuth env", provider: "minimax",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_OAUTH,
				LookupEnv:      func(string) string { return "should-not-be-used" },
			},
			wantErr: "not OAuth-capable",
		},
		{
			name: "strict api_key rejects provider without API key env", provider: "codex",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_APIKEY,
				LookupEnv:      func(string) string { return "" },
			},
			wantErr: "OAuth-only",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Assert on the resolution itself rather than on a built
			// provider: constructing one may perform network-free but
			// credential-dependent validation we do not want to couple to.
			e, ok := provider.Lookup(tc.provider)
			require.True(t, ok)

			got, err := tc.opts.Resolve(e.Metadata)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantAPIKey, got.APIKey)
			if tc.wantBaseURL != "" {
				assert.Equal(t, tc.wantBaseURL, got.BaseURL)
			}
		})
	}
}

func TestNewBuildsAProvider(t *testing.T) {
	// ollama is keyless, so it constructs without any credential present.
	p, err := provider.New("ollama", provider.Options{
		BaseURL:   "http://localhost:11434/v1",
		LookupEnv: func(string) string { return "" },
	})
	require.NoError(t, err)
	require.NotNil(t, p)

	var runtimeProvider core.Provider = p
	var streamProvider core.StreamProvider = p
	assert.NotNil(t, runtimeProvider)
	assert.NotNil(t, streamProvider)

	entry, ok := provider.Lookup("ollama")
	require.True(t, ok)
	assert.NotEmpty(t, entry.Metadata.Label, "the registry entry owns discovery metadata")
	assert.NotEmpty(t, entry.Catalog(), "the registry entry owns the static catalog")
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	// A second registration with the same Name must panic at init time;
	// we exercise it directly to verify the invariant.
	assert.Panics(t, func() {
		provider.Register(provider.Entry{Name: "minimax", New: func(provider.Options) (provider.Adapter, error) {
			return nil, nil
		}})
	}, "Register must reject duplicate names")
}

func TestRegisterRejectsIncompleteEntry(t *testing.T) {
	// An Entry without Name or New is a programmer error and must panic
	// rather than silently produce a half-usable registration. The panic
	// message includes a dump of the offending entry, so we recover and
	// assert on a substring rather than the full string.
	cases := []struct {
		name  string
		entry provider.Entry
	}{
		{"missing New", provider.Entry{Name: "incomplete-only-name"}},
		{"missing Name", provider.Entry{New: func(provider.Options) (provider.Adapter, error) {
			return nil, nil
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				require.NotNil(t, r, "Register must panic on incomplete entry")
				msg := strings.TrimSpace(fmt.Sprint(r))
				assert.Containsf(t, msg, "provider: Register requires Name and New",
					"panic message %q should mention the missing-field invariant", msg)
			}()
			provider.Register(tc.entry)
		})
	}
}
