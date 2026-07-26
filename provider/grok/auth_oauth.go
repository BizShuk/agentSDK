package grok

import "time"

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
// shape of a token it is handed. Everything needed to talk to the API;
// nothing needed to obtain a credential.

// OAuthCredentials is an OAuth token as this adapter receives it.
//
// It is a transport DTO, not a stored credential: the authoritative
// record lives in the auth module's model.Credential.
type OAuthCredentials struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
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
