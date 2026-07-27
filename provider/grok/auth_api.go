// Auth helpers for the API-key (long-lived secret) flavor of xAI Grok.
//
// xAI documents this flow at https://docs.x.ai/docs/authentication —
// generate a key in the xAI console and pass it via XAI_API_KEY.

package grok

const (
	// APIKeyEnvVar is the standard env var for an xAI API key.
	APIKeyEnvVar = "XAI_API_KEY"

	// BaseURLEnvVar is the standard env var for overriding the Grok API
	// endpoint (e.g. for a corporate proxy or local mock).
	BaseURLEnvVar = "XAI_BASE_URL"

	// DefaultBaseURL is the public xAI chat-completions endpoint.
	DefaultBaseURL = "https://api.x.ai/v1"
)
