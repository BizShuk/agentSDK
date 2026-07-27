package antigravity

const (
	// APIKeyEnvVar is the env var for a direct Antigravity API key path.
	// Some deployments expose an API key in addition to the OAuth flow.
	APIKeyEnvVar = "ANTIGRAVITY_API_KEY"

	// BaseURLEnvVar overrides the default Antigravity gateway URL.
	BaseURLEnvVar = "ANTIGRAVITY_BASE_URL"

	// DefaultBaseURL is Google's Antigravity gateway. Confirm against
	// https://help.router-for-me/configuration/provider/antigravity once
	// live — the gateway URL may differ.
	DefaultBaseURL = "https://antigravity.googleapis.com/v1"
)
