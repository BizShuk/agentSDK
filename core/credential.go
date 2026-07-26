package core

// CredentialKind selects which credential class a strict lookup
// consults. It is core vocabulary rather than provider vocabulary
// because these are exactly the values Provider.AuthSchemes reports —
// the port is defined here, so the words it speaks are defined here too.
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
