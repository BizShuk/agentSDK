package codex

import (
	"fmt"
	"runtime"
	"time"
)

// OAuth support for this adapter is limited to ACCEPTING a token.
//
// The flow that produces one — PKCE, the authorize URL, the code
// exchange, the local callback server, opening a browser, refreshing —
// lives in the auth module and is reached through provider/credential.
// This file used to carry its own copy of all of it; four adapters did,
// and the copies had already drifted from auth (and from each other) on
// token endpoints, redirect URIs, and scopes, with no caller on any side
// to say which was right.
//
// What stays here is what the adapter itself needs at REQUEST time: the
// shape of a token it is handed, and the client identity the endpoint
// insists on. Everything needed to talk to the API; nothing needed to
// obtain a credential.

const (
	// CodexOriginator identifies requests made through the Codex
	// adapter. The upstream /codex/responses endpoint rejects requests
	// that do not carry this header.
	CodexOriginator = "codex_cli_rs"

	// CodexVersion is the version string the chatgpt.com endpoint
	// expects. Mismatches fall through to a 403 in upstream's bot
	// detection.
	CodexVersion = "0.125.0"
)

// OAuthCredentials is an OAuth token as this adapter receives it.
//
// AccountID is carried because Codex identifies the paying account with
// a header rather than with the token; a valid token without it still
// 401s. It is a transport DTO — the authoritative record lives in the
// auth module's model.Credential.
type OAuthCredentials struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	AccountID    string
}

// IsExpired reports whether the token needs a refresh. The 60s grace
// window keeps a token from expiring between the check and the call it
// authorizes.
func (c OAuthCredentials) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Until(c.ExpiresAt) < 60*time.Second
}

// CodexUserAgent builds the User-Agent the endpoint expects. The
// platform and architecture tokens mirror the Rust CLI's format; the
// endpoint's bot detection reads them.
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
