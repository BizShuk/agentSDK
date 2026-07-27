package provider

import (
	"fmt"
	"os"

	"github.com/bizshuk/agentsdk/core"
)

// Options are the per-construction overrides. Every field is optional:
// an empty field means "let the adapter apply its own environment
// fallback", which is why a zero Options still builds a working provider
// on a machine with the credentials exported.
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

// Resolve fills empty credential fields from the environment, honouring
// the strict modes in Options.CredentialKind. The returned Options is
// always usable; the error is non-nil only when a strict mode rejects
// the resolved state.
//
// Precedence (any mode):
//  1. An explicit Options.APIKey wins outright, before any env lookup.
//  2. Otherwise the chosen env class supplies the key.
//  3. Options.BaseURL still resolves from Metadata.BaseURLEnv, independent of CredentialKind.
//
// Exported because "which credential would this actually use" is a
// question callers legitimately ask before building anything — a preflight
// check, or a wizard showing which env var it found.
func (o Options) Resolve(m Metadata) (Options, error) {
	if o.APIKey == "" {
		switch o.CredentialKind {
		case core.CREDENTIAL_KIND_OAUTH:
			if len(m.OAuthEnv) == 0 {
				return o, fmt.Errorf("provider %q: not OAuth-capable (no OAuth env registered)", m.Label)
			}
			if v := o.firstEnv(m.OAuthEnv, o.APIKeyEnv); v != "" {
				o.APIKey = v
			} else {
				return o, fmt.Errorf("provider %q: requires OAuth credential but %v is unset",
					m.Label, m.OAuthEnv)
			}
		case core.CREDENTIAL_KIND_APIKEY:
			if len(m.APIKeyEnv) == 0 {
				return o, fmt.Errorf("provider %q: OAuth-only, does not accept api_key credential", m.Label)
			}
			if v := o.firstEnv(m.APIKeyEnv, o.APIKeyEnv); v != "" {
				o.APIKey = v
			} else {
				return o, fmt.Errorf("provider %q: requires api_key credential but %v is unset",
					m.Label, m.APIKeyEnv)
			}
		default: // "" / "auto" — preserve the legacy precedence
			keys := append(append([]string{}, m.OAuthEnv...), m.APIKeyEnv...)
			if v := o.firstEnv(keys, o.APIKeyEnv); v != "" {
				o.APIKey = v
			}
		}
	}
	if o.BaseURL == "" {
		o.BaseURL = o.lookup(m.BaseURLEnv)
	}
	return o, nil
}
