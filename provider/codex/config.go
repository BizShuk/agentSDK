package codex

import "github.com/bizshuk/agentsdk/provider/utils"

// Codex configuration is OAuth-first — most users authenticate via ChatGPT Plus/Pro
// credentials resolved by provider/credential. The API key path is provided
// for completeness and local mocks that do not require the OAuth handshake.

const (
	// APIKeyEnvVar — placeholder; the Codex endpoint does not accept
	// arbitrary OpenAI keys. Most deployments should use the OAuth
	// credential flow instead.
	APIKeyEnvVar = "OPENAI_API_KEY"

	// OAuthEnvVar is the env var for a pre-issued OpenAI OAuth access token.
	OAuthEnvVar = "OPENAI_OAUTH_TOKEN"

	// BaseURLEnvVar — override the upstream base URL. Useful for
	// pointing tests at a local mock.
	BaseURLEnvVar = "CODEX_BASE_URL"

	// DefaultBaseURL is the production Codex endpoint. Code behind
	// the chatgpt.com boundary — there is no api.openai.com alias.
	DefaultBaseURL = "https://chatgpt.com/backend-api"

	// PATH_RESPONSES is the Responses-shaped generation endpoint. It is not
	// an alias of OpenAI's public /v1/responses: the request is rejected if
	// it carries max_output_tokens, and store must be false.
	PATH_RESPONSES = "/codex/responses"

	// LiveBaseURLEnvVar overrides the realtime websocket endpoint used by
	// the Live API surface.
	LiveBaseURLEnvVar = "CODEX_LIVE_BASE_URL"

	// DefaultLiveBaseURL is the OpenAI Realtime API websocket. Unlike the
	// chat surface this lives on api.openai.com and accepts standard API
	// keys — the one surface where APIKeyEnvVar is not a placeholder.
	DefaultLiveBaseURL = "wss://api.openai.com/v1/realtime"

	// DefaultLiveModel is the GA realtime speech model.
	DefaultLiveModel = "gpt-realtime"

	// DefaultInputTranscriptionModel transcribes user audio when the
	// session asks for input transcripts; the realtime session config
	// requires an explicit model name.
	DefaultInputTranscriptionModel = "whisper-1"

	// CodexOriginator identifies requests made through the Codex adapter.
	CodexOriginator = "codex_cli_rs"

	// CodexVersion is the version string the chatgpt.com endpoint expects.
	// It must be at least as new as the version upstream enforces for the
	// newest models — gpt-5.6-sol answers 400 to an older one.
	CodexVersion = "0.144.1"
)

// Client identity headers. The endpoint gates on these alongside the
// bearer token, and answers 400 for an unrecognised originator/version pair.
const (
	OriginatorHeader = "originator"
	VersionHeader    = "version"

	// AccountIDHeader selects which ChatGPT account a multi-account token
	// bills against. Absent, the endpoint picks the token's default.
	AccountIDHeader = "ChatGPT-Account-ID"
)

// CodexUserAgent builds the User-Agent the endpoint expects.
func CodexUserAgent() string {
	return utils.CLIUserAgent(CodexOriginator, CodexVersion)
}
