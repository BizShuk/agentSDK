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
