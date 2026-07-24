package registry_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamesAreSortedAndComplete(t *testing.T) {
	// The registered set depends on what the linking binary imports; the
	// test binary blank-imports provider/all (see providers_test.go), so
	// every built-in adapter must appear.
	got := registry.Names()
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

func TestSpecDefaultProviderIsRegistered(t *testing.T) {
	// spec is core-only and cannot import this package, so the two
	// default strings can only be kept in step by a test.
	_, ok := registry.Lookup(spec.DEFAULT_PROVIDER)
	assert.Truef(t, ok, "spec.DEFAULT_PROVIDER %q is not a registered provider", spec.DEFAULT_PROVIDER)
	assert.Equal(t, registry.DEFAULT, spec.DEFAULT_PROVIDER)
}

func TestLookupNormalizesName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"minimax", "minimax"},
		{"MiniMax", "minimax"},
		{"  ANTHROPIC  ", "anthropic"},
		{"", registry.DEFAULT},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			e, ok := registry.Lookup(tc.in)
			require.True(t, ok)
			assert.Equal(t, tc.want, e.Name)
		})
	}
}

func TestLookupUnknown(t *testing.T) {
	_, ok := registry.Lookup("bogus")
	assert.False(t, ok)

	_, err := registry.New("bogus", registry.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
	assert.Contains(t, err.Error(), "minimax", "the error should list what IS registered")
}

func TestEveryEntryIsSelfDescribing(t *testing.T) {
	for _, e := range registry.Entries() {
		t.Run(e.Name, func(t *testing.T) {
			assert.NotEmpty(t, e.Name)
			assert.NotEmpty(t, e.Label, "a wizard menu renders Label")
			// API-key paths must name the env var; OAuth-only entries
			// document themselves in Note instead.
			oauthOnly := strings.Contains(strings.ToLower(e.Note), "oauth")
			if !oauthOnly {
				assert.NotEmptyf(t, e.APIKeyEnv,
					"every non-OAuth entry must document how its credential resolves; got %+v", e)
			}
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
		opts        registry.Options
		wantAPIKey  string
		wantBaseURL string
	}{
		{
			name: "oauth outranks api key", provider: "anthropic",
			opts:       registry.Options{LookupEnv: lookup},
			wantAPIKey: "from-oauth",
		},
		{
			name: "explicit key outranks env", provider: "anthropic",
			opts:       registry.Options{APIKey: "explicit", LookupEnv: lookup},
			wantAPIKey: "explicit",
		},
		{
			name: "base url from env", provider: "minimax",
			opts:        registry.Options{LookupEnv: lookup},
			wantAPIKey:  "mini-key",
			wantBaseURL: "https://example.test/v1",
		},
		{
			name: "APIKeyEnv override redirects the lookup", provider: "minimax",
			opts:       registry.Options{APIKeyEnv: "ANTHROPIC_API_KEY", LookupEnv: lookup},
			wantAPIKey: "from-api-key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Assert on the resolution itself rather than on a built
			// provider: constructing one may perform network-free but
			// credential-dependent validation we do not want to couple to.
			e, ok := registry.Lookup(tc.provider)
			require.True(t, ok)

			got := tc.opts.Resolve(e)
			assert.Equal(t, tc.wantAPIKey, got.APIKey)
			if tc.wantBaseURL != "" {
				assert.Equal(t, tc.wantBaseURL, got.BaseURL)
			}
		})
	}
}

func TestNewBuildsAProvider(t *testing.T) {
	// ollama is keyless, so it constructs without any credential present.
	p, err := registry.New("ollama", registry.Options{
		BaseURL:   "http://localhost:11434/v1",
		LookupEnv: func(string) string { return "" },
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NotEmpty(t, p.ID())
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	// A second registration with the same Name must panic at init time;
	// we exercise it directly to verify the invariant.
	assert.Panics(t, func() {
		registry.Register(registry.Entry{Name: "minimax", New: func(registry.Options) (core.Provider, error) {
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
		entry registry.Entry
	}{
		{"missing New", registry.Entry{Name: "incomplete-only-name"}},
		{"missing Name", registry.Entry{New: func(registry.Options) (core.Provider, error) {
			return nil, nil
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				require.NotNil(t, r, "Register must panic on incomplete entry")
				msg := strings.TrimSpace(fmt.Sprint(r))
				assert.Containsf(t, msg, "registry: Register requires Name and New",
					"panic message %q should mention the missing-field invariant", msg)
			}()
			registry.Register(tc.entry)
		})
	}
}
