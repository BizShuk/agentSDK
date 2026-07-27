package provider

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/core"
)

// Options are unresolved live inputs owned by the provider registry. Every
// field is optional; Resolve combines them with Entry.Metadata before an
// adapter sees the construction request.
type Options struct {
	Model   string
	APIKey  string
	BaseURL string

	// APIKeyEnv overrides which environment variable supplies the
	// credential, for deployments that do not use the adapter's
	// conventional name. Empty uses the entry's own list.
	APIKeyEnv string

	// LookupEnv resolves an environment variable. nil means os.Getenv.
	// The CLI passes a viper-backed lookup so .env files participate.
	LookupEnv func(string) string

	// Decorator resolves the credential before every outbound request.
	// nil keeps the construction-time credential, which is what the env
	// path wants: an API key read from the environment does not expire.
	// provider/credential supplies one when the application stores
	// credentials rather than exporting them.
	Decorator Decorator

	// CredentialKind selects which credential class is consulted. The
	// values are core.CREDENTIAL_KIND_* — they are core vocabulary
	// shared with the declarative agent/spec model.
	//
	// AUTO ("") tries OAuthEnv first, then APIKeyEnv (the legacy
	// precedence). OAUTH restricts to OAuthEnv and returns an error when
	// no OAuth env resolves. APIKEY restricts to APIKeyEnv and returns an
	// error when no API key env resolves. The strict modes catch the case
	// where a stale OAuth token outranks a fresh API key (or vice versa)
	// on a shared machine.
	CredentialKind string
}

// ResolvedConfig is the complete construction input handed to an adapter.
// Unlike Options, it contains no environment lookup or request decorator:
// both concerns are consumed by the registry before the factory is called.
type ResolvedConfig struct {
	Model   string
	BaseURL string
	Auth    core.Auth
}

func (o Options) lookup(key string) string {
	if key == "" {
		return ""
	}
	if o.LookupEnv != nil {
		return o.LookupEnv(key)
	}
	return os.Getenv(key)
}

// firstEnv returns the first non-empty value found in keys, or "" when
// none resolve. The override (when set) restricts the search to that
// single key.
func (o Options) firstEnv(keys []string, override string) string {
	if override != "" {
		return o.lookup(override)
	}
	for _, k := range keys {
		if v := o.lookup(k); v != "" {
			return v
		}
	}
	return ""
}

// Resolve turns unresolved options into the single construction config handed
// to an adapter. OAuth environment values become Auth.Bearer; API-key
// environment values become Auth.APIKey.
//
// Precedence (any mode):
//  1. An explicit Options.APIKey wins outright, before any env lookup.
//  2. A Decorator defers credential resolution to each outbound request.
//  3. Otherwise the chosen env class supplies the construction credential.
//  4. Options.BaseURL still resolves from Metadata.BaseURLEnv, independent of CredentialKind.
//
// Exported because "which credential would this actually use" is a
// question callers legitimately ask before building anything — a preflight
// check, or a wizard showing which env var it found.
func (o Options) Resolve(m Metadata) (ResolvedConfig, error) {
	resolved := ResolvedConfig{
		Model:   o.Model,
		BaseURL: o.BaseURL,
	}
	if o.APIKey != "" {
		resolved.Auth.APIKey = o.APIKey
	} else if o.Decorator == nil {
		switch o.CredentialKind {
		case core.CREDENTIAL_KIND_OAUTH:
			if len(m.OAuthEnv) == 0 {
				return resolved, fmt.Errorf("provider %q: not OAuth-capable (no OAuth env registered)", m.Label)
			}
			if v := o.firstEnv(m.OAuthEnv, o.APIKeyEnv); v != "" {
				resolved.Auth.Bearer = v
			} else {
				return resolved, fmt.Errorf("provider %q: requires OAuth credential but %v is unset",
					m.Label, m.OAuthEnv)
			}
		case core.CREDENTIAL_KIND_APIKEY:
			if len(m.APIKeyEnv) == 0 {
				return resolved, fmt.Errorf("provider %q: OAuth-only, does not accept api_key credential", m.Label)
			}
			if v := o.firstEnv(m.APIKeyEnv, o.APIKeyEnv); v != "" {
				resolved.Auth.APIKey = v
			} else {
				return resolved, fmt.Errorf("provider %q: requires api_key credential but %v is unset",
					m.Label, m.APIKeyEnv)
			}
		default: // "" / "auto" — OAuth env outranks API-key env
			if o.APIKeyEnv != "" {
				resolved.Auth.APIKey = o.lookup(o.APIKeyEnv)
				break
			}
			if v := o.firstEnv(m.OAuthEnv, ""); v != "" {
				resolved.Auth.Bearer = v
			} else if v := o.firstEnv(m.APIKeyEnv, ""); v != "" {
				resolved.Auth.APIKey = v
			}
		}
	}
	if resolved.BaseURL == "" {
		resolved.BaseURL = o.lookup(m.BaseURLEnv)
	}
	return resolved, nil
}
