package minimax

// minimax accepts long-lived API keys only — no OAuth. The auth header
// uses Anthropic's convention `x-api-key: <key>` rather than the more
// common `Authorization: Bearer` because the underlying endpoint is an
// Anthropic-Messages-compat surface.

// APIKeyEnvVar is the standard env var for a minimax API key.
const APIKeyEnvVar = "MINIMAX_API_KEY"

// BaseURLEnvVar lets operators point at a self-hosted or proxy-fronted
// minimax endpoint without recompiling.
const BaseURLEnvVar = "MINIMAX_BASE_URL"

// DefaultBaseURL is the public minimax Anthropic-compat endpoint.
const DefaultBaseURL = "https://api.minimax.io/anthropic"
