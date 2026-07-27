package antigravity

const (
	// APIKeyEnvVar configures a direct Antigravity API key path.
	// Some deployments expose an API key in addition to the OAuth flow.
	APIKeyEnvVar = "ANTIGRAVITY_API_KEY"

	// OAuthEnvVar configures a pre-issued Antigravity OAuth access token path.
	OAuthEnvVar = "ANTIGRAVITY_OAUTH_TOKEN"

	// BaseURLEnvVar overrides the default Antigravity gateway URL.
	BaseURLEnvVar = "ANTIGRAVITY_BASE_URL"

	// DefaultBaseURL is Google's Antigravity gateway. Confirm against
	// https://help.router-for-me/configuration/provider/antigravity once
	// live — the gateway URL may differ.
	DefaultBaseURL = "https://antigravity.googleapis.com/v1"
)
