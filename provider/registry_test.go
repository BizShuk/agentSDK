package provider_test

import (
	"context"
	"errors"
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
			// APIKeyEnv, or a decorator-based path (e.g. codex's OAuth
			// is provided by provider/credential, not an env var). Entries that
			// declare NONE of the above have no way to bootstrap a
			// credential through the registry at all.
			hasEnvPath := len(e.Metadata.OAuthEnv) > 0 || len(e.Metadata.APIKeyEnv) > 0
			hasDecoratorPath := strings.Contains(strings.ToLower(e.Metadata.Note), "decorator")
			assert.Truef(t, hasEnvPath || hasDecoratorPath,
				"entry %q has no credential path (OAuthEnv=%v APIKeyEnv=%v Note=%q); declare an env OR document a decorator path",
				e.Name, e.Metadata.OAuthEnv, e.Metadata.APIKeyEnv, e.Metadata.Note)
			assert.True(t,
				e.New != nil || e.NewImage != nil || e.NewVideo != nil ||
					e.NewMusic != nil || e.NewTranscriber != nil || e.NewSpeech != nil,
				"every entry must expose at least one factory")
			if e.New != nil {
				assert.NotNil(t, e.Catalog,
					"every model-capable entry must expose its bundled catalog")
			}
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
		"OPENAI_API_KEY":        "openai-key",
	}
	lookup := func(k string) string { return env[k] }

	cases := []struct {
		name        string
		provider    string
		opts        provider.Options
		wantAuth    core.Auth
		wantBaseURL string
		wantErr     string
	}{
		{
			name: "oauth outranks api key", provider: "anthropic",
			opts:     provider.Options{LookupEnv: lookup},
			wantAuth: core.Auth{Bearer: "from-oauth"},
		},
		{
			name: "explicit key outranks env", provider: "anthropic",
			opts:     provider.Options{APIKey: "explicit", LookupEnv: lookup},
			wantAuth: core.Auth{APIKey: "explicit"},
		},
		{
			name: "base url from env", provider: "minimax",
			opts:        provider.Options{LookupEnv: lookup},
			wantAuth:    core.Auth{APIKey: "mini-key"},
			wantBaseURL: "https://example.test/v1",
		},
		{
			name: "APIKeyEnv override redirects the lookup", provider: "minimax",
			opts:     provider.Options{APIKeyEnv: "ANTHROPIC_API_KEY", LookupEnv: lookup},
			wantAuth: core.Auth{APIKey: "from-api-key"},
		},
		{
			name: "codex API key env resolves in registry", provider: "codex",
			opts:     provider.Options{LookupEnv: lookup},
			wantAuth: core.Auth{APIKey: "openai-key"},
		},
		{
			name: "strict oauth picks OAuth env over API key", provider: "anthropic",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_OAUTH,
				LookupEnv:      lookup,
			},
			wantAuth: core.Auth{Bearer: "from-oauth"},
		},
		{
			name: "strict api_key ignores OAuth env", provider: "anthropic",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_APIKEY,
				LookupEnv:      lookup,
			},
			wantAuth: core.Auth{APIKey: "from-api-key"},
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
			name: "strict oauth rejects provider without OAuth env", provider: "google",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_OAUTH,
				LookupEnv:      func(string) string { return "should-not-be-used" },
			},
			wantErr: "not OAuth-capable",
		},
		{
			name: "strict api_key requires configured env", provider: "codex",
			opts: provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_APIKEY,
				LookupEnv:      func(string) string { return "" },
			},
			wantErr: "requires api_key credential",
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
			assert.Equal(t, tc.wantAuth, got.Auth)
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

func TestImageCapabilitiesAreExplicit(t *testing.T) {
	// antigravity has no image endpoint; it reaches the capability through
	// its own chat surface, which is exactly what the Entry factory is for.
	for _, name := range []string{"google", "grok", "minimax", "antigravity"} {
		t.Run(name, func(t *testing.T) {
			entry, ok := provider.Lookup(name)
			require.True(t, ok)
			assert.True(t, entry.Supports(provider.CAPABILITY_IMAGE))
			assert.Contains(t, entry.Capabilities(), provider.CAPABILITY_IMAGE)
		})
	}
	for _, name := range []string{"anthropic", "codex", "ollama"} {
		t.Run(name, func(t *testing.T) {
			entry, ok := provider.Lookup(name)
			require.True(t, ok)
			assert.False(t, entry.Supports(provider.CAPABILITY_IMAGE))
			assert.NotContains(t, entry.Capabilities(), provider.CAPABILITY_IMAGE)
		})
	}
}

func TestVideoCapabilitiesAreExplicit(t *testing.T) {
	entry, ok := provider.Lookup("minimax")
	require.True(t, ok)
	assert.True(t, entry.Supports(provider.CAPABILITY_VIDEO))
	assert.Contains(t, entry.Capabilities(), provider.CAPABILITY_VIDEO)

	for _, name := range []string{"anthropic", "antigravity", "codex", "google", "grok", "ollama"} {
		t.Run(name, func(t *testing.T) {
			entry, ok := provider.Lookup(name)
			require.True(t, ok)
			assert.False(t, entry.Supports(provider.CAPABILITY_VIDEO))
			assert.NotContains(t, entry.Capabilities(), provider.CAPABILITY_VIDEO)
		})
	}
}

func TestMusicCapabilitiesAreExplicit(t *testing.T) {
	entry, ok := provider.Lookup("minimax")
	require.True(t, ok)
	assert.True(t, entry.Supports(provider.CAPABILITY_MUSIC))
	assert.Contains(t, entry.Capabilities(), provider.CAPABILITY_MUSIC)

	for _, name := range []string{"anthropic", "antigravity", "codex", "google", "grok", "ollama"} {
		t.Run(name, func(t *testing.T) {
			entry, ok := provider.Lookup(name)
			require.True(t, ok)
			assert.False(t, entry.Supports(provider.CAPABILITY_MUSIC))
			assert.NotContains(t, entry.Capabilities(), provider.CAPABILITY_MUSIC)
		})
	}
}

func TestNewImageRejectsUnsupportedCapabilityBeforeCredentialResolution(t *testing.T) {
	_, err := provider.NewImage("anthropic", provider.Options{
		LookupEnv: func(string) string { return "" },
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrUnsupportedCapability)

	var unsupported *provider.UnsupportedCapabilityError
	require.True(t, errors.As(err, &unsupported))
	assert.Equal(t, "anthropic", unsupported.Provider)
	assert.Equal(t, provider.CAPABILITY_IMAGE, unsupported.Capability)
}

func TestNewImageAllowsDeferredCredentialConstruction(t *testing.T) {
	for _, name := range []string{"google", "grok"} {
		t.Run(name, func(t *testing.T) {
			generator, err := provider.NewImage(name, provider.Options{
				LookupEnv: func(string) string { return "" },
				Decorator: func(context.Context) (core.Auth, error) {
					return core.Auth{Bearer: "resolved-per-request"}, nil
				},
			})
			require.NoError(t, err)
			assert.NotNil(t, generator)
		})
	}
}

func TestNewVideoRejectsUnsupportedCapabilityBeforeCredentialResolution(t *testing.T) {
	_, err := provider.NewVideo("anthropic", provider.Options{
		LookupEnv: func(string) string { return "" },
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrUnsupportedCapability)

	var unsupported *provider.UnsupportedCapabilityError
	require.True(t, errors.As(err, &unsupported))
	assert.Equal(t, "anthropic", unsupported.Provider)
	assert.Equal(t, provider.CAPABILITY_VIDEO, unsupported.Capability)
}

func TestNewMusicRejectsUnsupportedCapabilityBeforeCredentialResolution(t *testing.T) {
	_, err := provider.NewMusic("anthropic", provider.Options{
		LookupEnv: func(string) string { return "" },
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, provider.ErrUnsupportedCapability)

	var unsupported *provider.UnsupportedCapabilityError
	require.True(t, errors.As(err, &unsupported))
	assert.Equal(t, "anthropic", unsupported.Provider)
	assert.Equal(t, provider.CAPABILITY_MUSIC, unsupported.Capability)
}

func TestNewMusicAllowsDeferredCredentialConstruction(t *testing.T) {
	generator, err := provider.NewMusic("minimax", provider.Options{
		LookupEnv: func(string) string { return "" },
		Decorator: func(context.Context) (core.Auth, error) {
			return core.Auth{Bearer: "resolved-per-request"}, nil
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, generator)
}

func TestDecoratorAllowsDeferredCredentialConstruction(t *testing.T) {
	for _, entry := range provider.Entries() {
		// Audio-only entries have no model factory to defer a credential
		// into; NewSpeech / NewTranscriber cover them.
		if !entry.Metadata.CredentialRequired || entry.New == nil {
			continue
		}
		t.Run(entry.Name, func(t *testing.T) {
			p, err := provider.New(entry.Name, provider.Options{
				CredentialKind: core.CREDENTIAL_KIND_OAUTH,
				LookupEnv:      func(string) string { return "" },
				Decorator: func(context.Context) (core.Auth, error) {
					return core.Auth{Bearer: "resolved-per-request"}, nil
				},
			})
			require.NoError(t, err)
			assert.NotNil(t, p)
		})
	}
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	// A second registration with the same Name must panic at init time;
	// we exercise it directly to verify the invariant.
	assert.Panics(t, func() {
		provider.Register(provider.Entry{Name: "minimax", New: func(provider.ResolvedConfig) (provider.Adapter, error) {
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
		{"missing factories", provider.Entry{Name: "incomplete-only-name"}},
		{"missing Name", provider.Entry{New: func(provider.ResolvedConfig) (provider.Adapter, error) {
			return nil, nil
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				require.NotNil(t, r, "Register must panic on incomplete entry")
				msg := strings.TrimSpace(fmt.Sprint(r))
				assert.Containsf(t, msg, "provider: Register requires Name and at least one factory",
					"panic message %q should mention the missing-field invariant", msg)
			}()
			provider.Register(tc.entry)
		})
	}
}
