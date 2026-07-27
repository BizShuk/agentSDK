package core

// CredentialKind selects which credential class a strict lookup
// consults. It is core vocabulary rather than provider vocabulary
// because agent/spec must describe credential intent without importing
// provider discovery or authentication mechanisms.
//
// CREDENTIAL_KIND_AUTO (the empty string) preserves the legacy
// precedence: OAuth outranks API key, first non-empty wins. The two
// strict modes restrict the lookup to one class and fail at resolution
// time when nothing resolves, which is what catches a stale OAuth token
// outranking a fresh API key on a shared machine.
//
// The provider name a run defaults to is NOT here: a vendor name in core
// would mean the pure state machine changes when the default vendor
// does. It lives in provider.DEFAULT_NAME, next to the registry that can
// actually answer which adapters are linked in.
const (
	CREDENTIAL_KIND_AUTO   = ""
	CREDENTIAL_KIND_APIKEY = "api_key"
	CREDENTIAL_KIND_OAUTH  = "oauth"
)

// Auth carries the resolved credentials for a single request. Callers
// populate this before calling Provider.Generate or StreamProvider.Stream;
// the provider itself does not reach out to fetch credentials.
//
// At most one of APIKey or Bearer should be set. Headers and BaseURL are
// optional overrides.
type Auth struct {
	// APIKey is sent as `x-api-key: <value>` (Anthropic-style) or
	// `Authorization: Bearer <value>` (OpenAI-style) depending on the
	// provider. Empty when using OAuth.
	APIKey string `json:"api_key,omitempty"`

	// Bearer is the OAuth access token. Empty when using an API key.
	Bearer string `json:"bearer,omitempty"`

	// Headers carries provider-specific overrides (e.g. anthropic-beta,
	// ChatGPT-Account-ID). Merged on top of provider defaults.
	Headers map[string]string `json:"headers,omitempty"`

	// BaseURL overrides the provider's default base URL. Empty means use the
	// provider default.
	BaseURL string `json:"base_url,omitempty"`
}

// IsZero reports whether the Auth carries no credential, header override, or
// endpoint override.
func (a Auth) IsZero() bool {
	return a.APIKey == "" && a.Bearer == "" && a.BaseURL == "" && len(a.Headers) == 0
}

// Merge returns a copy of a with every non-zero field of override applied on
// top. Neither receiver nor argument is mutated, so construction-time Auth
// remains safe to reuse across concurrent requests.
func (a Auth) Merge(override Auth) Auth {
	out := a
	if override.APIKey != "" {
		out.APIKey = override.APIKey
	}
	if override.Bearer != "" {
		out.Bearer = override.Bearer
	}
	if override.BaseURL != "" {
		out.BaseURL = override.BaseURL
	}
	if len(override.Headers) > 0 {
		merged := make(map[string]string, len(out.Headers)+len(override.Headers))
		for key, value := range out.Headers {
			merged[key] = value
		}
		for key, value := range override.Headers {
			merged[key] = value
		}
		out.Headers = merged
	}
	return out
}

// Token returns the credential that should be carried, preferring the OAuth
// access token.
func (a Auth) Token() string {
	if a.Bearer != "" {
		return a.Bearer
	}
	return a.APIKey
}
