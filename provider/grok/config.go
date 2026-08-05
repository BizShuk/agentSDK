// Configuration for both xAI Grok credential flavors.
//
// The API-key flow is documented at https://docs.x.ai/docs/authentication —
// generate a key in the xAI console and pass it via XAI_API_KEY. It talks to
// the public inference host.
//
// The OAuth flow is the one the Grok CLI uses. It is NOT the same endpoint:
// an OAuth access token is minted for cli-chat-proxy.grok.com and the public
// host refuses it, so the two flavors carry separate base URLs on purpose.
// Sending an OAuth token to DefaultBaseURL leaks a session credential to a
// host that was never issued it.

package grok

import "github.com/bizshuk/agentsdk/provider/utils"

const (
	// APIKeyEnvVar is the standard env var for an xAI API key.
	APIKeyEnvVar = "XAI_API_KEY"

	// OAuthEnvVar is the standard env var for an xAI OAuth access token.
	OAuthEnvVar = "XAI_OAUTH_TOKEN"

	// BaseURLEnvVar is the standard env var for overriding the Grok API
	// endpoint (e.g. for a corporate proxy or local mock).
	BaseURLEnvVar = "XAI_BASE_URL"

	// DefaultBaseURL is the public xAI inference host, used by the
	// API-key flavor.
	DefaultBaseURL = "https://api.x.ai/v1"

	// APIBaseURL is the same host without the version segment, for
	// callers that carry fully-qualified paths of their own.
	APIBaseURL = "https://api.x.ai"

	// OAuthBaseURL is the Grok CLI chat proxy — the only host that
	// accepts an xAI OAuth access token for inference.
	OAuthBaseURL = "https://cli-chat-proxy.grok.com/v1"
)

// Inference paths on the OAuth host. Unlike the public API these are not
// prefixed with /v1: the version already rides in OAuthBaseURL, and the
// host serves three wire dialects side by side.
const (
	OAUTH_PATH_RESPONSES = "/responses"
	OAUTH_PATH_CHAT      = "/chat/completions"
	OAUTH_PATH_MESSAGES  = "/messages"
)

// Image generation is served from the public host under both credential
// flavors — an OAuth access token carries the api:access scope, so it is
// accepted here even though inference is not.
const (
	IMAGE_BASE_URL = APIBaseURL
	IMAGE_PATH     = "/v1/images/generations"

	// DefaultImageModel is the image model used when a request names none.
	DefaultImageModel = "grok-imagine-image-quality"
)

// Client identity headers for the OAuth flavor. cli-chat-proxy gates on
// these in addition to the bearer token; a request missing the token-auth
// pair is answered 401 even with a valid token.
const (
	TokenAuthHeader = "X-XAI-Token-Auth"
	TokenAuthValue  = "xai-grok-cli"

	AuthenticateResponseHeader = "x-authenticateresponse"
	AuthenticateResponseValue  = "authenticate-response"

	ClientVersionHeader    = "x-grok-client-version"
	ClientIdentifierHeader = "x-grok-client-identifier"
	ClientModeHeader       = "x-grok-client-mode"

	// ClientVersion is the Grok CLI version cli-chat-proxy expects.
	ClientVersion = "0.2.112"

	// DefaultClientMode marks a non-interactive caller.
	DefaultClientMode = "headless"
)

// Per-request tracking headers. The host correlates a tool loop by these;
// a caller that omits them gets one conversation per request.
const (
	RequestIDHeader      = "x-grok-req-id"
	ConversationIDHeader = "x-grok-conv-id"
	SessionIDHeader      = "x-grok-session-id"
	TurnIndexHeader      = "x-grok-turn-idx"
	AgentIDHeader        = "x-grok-agent-id"
	DeploymentIDHeader   = "x-grok-deployment-id"
	ModelOverrideHeader  = "x-grok-model-override"

	// UserIDHeader carries the credential's account id. It is never
	// caller-supplied — the account is a property of the token.
	UserIDHeader = "x-grok-user-id"
)

// Response metadata the host returns alongside a completion.
const (
	ContextWindowHeader       = "x-grok-context-window"
	MaxCompletionTokensHeader = "x-grok-max-completion-tokens"
	ModelsETagHeader          = "x-models-etag"
	ShouldRetryHeader         = "x-should-retry"
)

// DefaultMaxTokens is the ceiling cli-chat-proxy assumes when an
// Anthropic-Messages request omits max_tokens, which that dialect
// requires but the Grok CLI does not always send.
const DefaultMaxTokens = 128_000

// UserAgent builds the User-Agent cli-chat-proxy expects. The identifier is
// the calling client's own name, echoed in x-grok-client-identifier — it is
// not an xAI-side value, so a host application passes its own name here.
func UserAgent(identifier string) string {
	return utils.CLIUserAgent(identifier, ClientVersion)
}
