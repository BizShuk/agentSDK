package codex

import (
	"fmt"
	"runtime"
)

// Codex is OAuth-first — most users authenticate via ChatGPT Plus/Pro
// credentials resolved by provider/credential. The API key path is provided
// for completeness and local mocks that do not require the OAuth handshake.

const (
	// APIKeyEnvVar — placeholder; the Codex endpoint does not accept
	// arbitrary OpenAI keys. Most deployments should use the OAuth
	// credential flow instead.
	APIKeyEnvVar = "OPENAI_API_KEY"

	// BaseURLEnvVar — override the upstream base URL. Useful for
	// pointing tests at a local mock.
	BaseURLEnvVar = "CODEX_BASE_URL"

	// DefaultBaseURL is the production Codex endpoint. Code behind
	// the chatgpt.com boundary — there is no api.openai.com alias.
	DefaultBaseURL = "https://chatgpt.com/backend-api"

	// CodexOriginator identifies requests made through the Codex adapter.
	CodexOriginator = "codex_cli_rs"

	// CodexVersion is the version string the chatgpt.com endpoint expects.
	CodexVersion = "0.125.0"
)

// CodexUserAgent builds the User-Agent the endpoint expects.
func CodexUserAgent() string {
	platform := "linux"
	switch runtime.GOOS {
	case "darwin":
		platform = "macos"
	case "windows":
		platform = "windows"
	}
	architecture := "x86_64"
	if runtime.GOARCH == "arm64" {
		architecture = "arm64"
	}
	return fmt.Sprintf("%s/%s (%s; %s)", CodexOriginator, CodexVersion, platform, architecture)
}
