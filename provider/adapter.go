package provider

import "github.com/bizshuk/agentsdk/core"

// Adapter is the paired model generate + stream capability contract.
//
// Runtime only consumes core.Provider. A registry entry that exposes model
// generation returns Adapter so the provider smoke-test CLI can also stream.
// Image-only entries can omit this capability. Discovery data belongs to
// Entry, not to a constructed adapter.
type Adapter interface {
	core.Provider
	core.StreamProvider
}

// Metadata describes a registry entry for credential resolution and CLI
// listing. Entry is its sole owner; constructed adapters carry capabilities,
// not discovery data.
//
// OAuthEnv and APIKeyEnv are split rather than packed into one
// priority list so callers can demand a strict credential class —
// `Options.CredentialKind = "oauth"` consults only OAuthEnv; "api_key"
// consults only APIKeyEnv. The empty / "auto" mode preserves the old
// precedence: OAuthEnv first, then APIKeyEnv. CredentialRequired keeps
// fail-fast validation at the registry boundary without forcing custom
// credentialless entries to opt out.
type Metadata struct {
	Label              string
	Note               string
	OAuthEnv           []string // OAuth token env list (highest precedence in auto mode)
	APIKeyEnv          []string // API key env list (lowest precedence in auto mode)
	BaseURLEnv         string
	VideoBaseURLEnv    string
	MusicBaseURLEnv    string
	CredentialRequired bool
}
