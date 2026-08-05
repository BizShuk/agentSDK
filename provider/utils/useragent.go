package utils

import (
	"fmt"
	"runtime"
)

// CLIUserAgent builds the `name/version (platform; arch)` User-Agent that
// the Rust-CLI-derived gateways (OpenAI Codex, xAI's cli-chat-proxy) match
// on. Both hosts parse the string rather than treating it as opaque, so the
// platform and architecture tokens are theirs, not Go's: "macos" not
// "darwin", "x86_64" not "amd64".
//
// The identifier is the CALLING client's own name. A gateway uses it to
// attribute traffic, so a host application should pass its own name rather
// than impersonate the reference CLI.
func CLIUserAgent(identifier, version string) string {
	return fmt.Sprintf("%s/%s (%s; %s)", identifier, version, cliPlatform(), cliArch())
}

func cliPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

func cliArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "x86_64"
}
