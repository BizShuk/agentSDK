package anthropic

// APIKeyEnvVar is the standard construction-time env var for an Anthropic API key.
const APIKeyEnvVar = "ANTHROPIC_API_KEY"

// APIKeyOAuthEnvVar is the env var for a pre-issued OAuth access token
// (used by Claude Code CLI forwarding and third-party proxies).
const APIKeyOAuthEnvVar = "ANTHROPIC_OAUTH_TOKEN"

// OAuthBetaHeader and OAuthBetaValue identify Anthropic OAuth requests.
// The adapter adds this header automatically whenever Auth.Bearer is set.
const (
	OAuthBetaHeader = "anthropic-beta"
	OAuthBetaValue  = "oauth-2025-04-20"
)

// DirectBrowserAccessHeader opts a request out of Anthropic's
// browser-origin refusal. The OAuth surface answers 403 without it because
// it assumes any bearer-token caller is a first-party web client.
const (
	DirectBrowserAccessHeader = "anthropic-dangerous-direct-browser-access"
	DirectBrowserAccessValue  = "true"
)

// APIVersionHeader and APIVersion pin the Messages wire contract. Anthropic
// dates its breaking changes rather than versioning the path, so every
// request must name the contract it was written against.
const (
	APIVersionHeader = "anthropic-version"
	APIVersion       = "2023-06-01"
)

// DefaultBaseURL is the public Anthropic API root.
const DefaultBaseURL = "https://api.anthropic.com"

// Endpoint paths on the Messages surface.
const (
	PATH_MESSAGES     = "/v1/messages"
	PATH_COUNT_TOKENS = "/v1/messages/count_tokens"
)
