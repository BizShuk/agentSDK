package antigravity

import (
	"fmt"
	"runtime"
)

// Antigravity is not a public API. It is Google's Cloud Code "v1internal"
// surface — the same host the Antigravity IDE talks to — and it accepts a
// Google OAuth access token only. There is no API-key form of the
// credential, which is why register.go declares no APIKeyEnv: an api_key
// credential kind is rejected by provider.Options.Resolve rather than sent
// and refused upstream.
//
// The wire contract below was reconstructed from two open proxies that
// front the same endpoint:
//
//	https://github.com/badrisnarayanan/antigravity-claude-proxy
//	https://github.com/frieser/antigravity-proxy
const (
	// OAuthEnvVar supplies a pre-issued Google OAuth access token. The
	// token must carry the cloud-platform scope the Antigravity client
	// requests; provider/credential mints one through the auth module.
	OAuthEnvVar = "ANTIGRAVITY_OAUTH_TOKEN"

	// BaseURLEnvVar pins the gateway root. Setting it also disables the
	// built-in host fallback: an operator who names a host means that
	// host, not "that host and then Google's".
	BaseURLEnvVar = "ANTIGRAVITY_BASE_URL"

	// ProjectIDEnvVar pins the Cloud Code project id, skipping the
	// loadCodeAssist discovery round-trip.
	ProjectIDEnvVar = "ANTIGRAVITY_PROJECT_ID"

	// DefaultBaseURL is the daily channel, which is what the IDE prefers
	// and where new models appear first.
	DefaultBaseURL = "https://daily-cloudcode-pa.googleapis.com"

	// FallbackBaseURL is the production channel, tried when the daily one
	// answers 403/404/5xx or refuses the connection.
	FallbackBaseURL = "https://cloudcode-pa.googleapis.com"

	// DefaultProjectID is the project the reference clients fall back to
	// when loadCodeAssist reports none. It is a Google-side sentinel, not
	// a per-user value.
	DefaultProjectID = "rising-fact-p41fc"

	// ClientVersion is the Antigravity client version reported in the
	// User-Agent and X-Client-Version headers.
	ClientVersion = "2.0.1"
)

// Endpoint paths on the gateway. Cloud Code is a gRPC-transcoded surface,
// so the method name rides in the path after a colon rather than as a
// separate path segment.
const (
	PATH_GENERATE         = "/v1internal:generateContent"
	PATH_STREAM           = "/v1internal:streamGenerateContent?alt=sse"
	PATH_MODELS           = "/v1internal:fetchAvailableModels"
	PATH_LOAD_CODE_ASSIST = "/v1internal:loadCodeAssist"
)

// Client identity headers. The gateway checks these; a request without
// them is answered 403 even with a valid token.
const (
	CLIENT_NAME     = "antigravity"
	GOOG_API_CLIENT = "gl-node/18.18.2 fire/0.8.6 grpc/1.10.x"

	// INTERLEAVED_THINKING_BETA is sent for Claude thinking models so the
	// model may reason between tool calls rather than only before the
	// first one.
	INTERLEAVED_THINKING_BETA = "interleaved-thinking-2025-05-14"
)

// Request shaping constants.
const (
	// DEFAULT_MAX_TOKENS applies when core.ModelRequest carries none.
	DEFAULT_MAX_TOKENS = 4096

	// GEMINI_MAX_OUTPUT_TOKENS is the ceiling the gateway enforces for
	// Gemini models regardless of what the catalog advertises. Sending
	// more is a 400, so we clamp instead.
	GEMINI_MAX_OUTPUT_TOKENS = 16384

	// CLAUDE_THINKING_BUDGET is the thinking budget sent for Claude
	// thinking models. The gateway ignores include_thoughts without a
	// budget and the model falls back to <thinking> tags inside plain
	// text, which would reach callers as ordinary assistant output.
	CLAUDE_THINKING_BUDGET = 32000

	// CLAUDE_THINKING_HEADROOM is added on top of the thinking budget
	// when the caller's max_tokens does not exceed it — the gateway
	// requires max_tokens > thinking_budget.
	CLAUDE_THINKING_HEADROOM = 8192

	// MAX_ERROR_BYTES caps how much of a non-2xx body is quoted back.
	MAX_ERROR_BYTES = 512

	// MAX_BODY_BYTES caps a buffered JSON response.
	MAX_BODY_BYTES = 8 << 20
)

// Cloud Code client metadata enums. The gateway takes these as numbers,
// not names — the strings only exist in the IDE's protobuf descriptors.
const (
	IDE_TYPE_ANTIGRAVITY = 9
	PLUGIN_TYPE_GEMINI   = 2

	PLATFORM_UNSPECIFIED   = 0
	PLATFORM_DARWIN_AMD64  = 1
	PLATFORM_DARWIN_ARM64  = 2
	PLATFORM_LINUX_AMD64   = 3
	PLATFORM_LINUX_ARM64   = 4
	PLATFORM_WINDOWS_AMD64 = 5
)

// UserAgent builds the client string the gateway matches on. Node reports
// x64 where Go reports amd64; the gateway sees the Node spelling from
// every real client, so we match it.
func UserAgent() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return fmt.Sprintf("%s/%s %s/%s", CLIENT_NAME, ClientVersion, runtime.GOOS, arch)
}

// platformEnum maps the running host onto the gateway's platform enum.
func platformEnum() int {
	arm := runtime.GOARCH == "arm64"
	switch runtime.GOOS {
	case "darwin":
		if arm {
			return PLATFORM_DARWIN_ARM64
		}
		return PLATFORM_DARWIN_AMD64
	case "linux":
		if arm {
			return PLATFORM_LINUX_ARM64
		}
		return PLATFORM_LINUX_AMD64
	case "windows":
		return PLATFORM_WINDOWS_AMD64
	}
	return PLATFORM_UNSPECIFIED
}
