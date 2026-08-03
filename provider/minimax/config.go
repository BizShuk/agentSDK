package minimax

// MiniMax configuration accepts long-lived API keys only — no OAuth. The auth header
// uses Anthropic's convention `x-api-key: <key>` rather than the more
// common `Authorization: Bearer` because the underlying endpoint is an
// Anthropic-Messages-compat surface.

// APIKeyEnvVar is the standard env var for a minimax API key.
const APIKeyEnvVar = "MINIMAX_API_KEY"

// OAuthEnvVar is the standard env var for a minimax OAuth access token.
const OAuthEnvVar = "MINIMAX_OAUTH_TOKEN"

// BaseURLEnvVar lets operators point at a self-hosted or proxy-fronted
// minimax endpoint without recompiling.
const BaseURLEnvVar = "MINIMAX_BASE_URL"

// VideoBaseURLEnvVar lets operators override the MiniMax video API root
// independently from the Anthropic-compatible model endpoint.
const VideoBaseURLEnvVar = "MINIMAX_VIDEO_BASE_URL"

// MusicBaseURLEnvVar lets operators override the MiniMax music API root
// independently from the Anthropic-compatible model endpoint.
const MusicBaseURLEnvVar = "MINIMAX_MUSIC_BASE_URL"

// SpeechBaseURLEnvVar lets operators override the MiniMax speech API root
// independently from the Anthropic-compatible model endpoint.
const SpeechBaseURLEnvVar = "MINIMAX_SPEECH_BASE_URL"

// DefaultBaseURL is the public minimax Anthropic-compat endpoint.
const DefaultBaseURL = "https://api.minimax.io/anthropic"

// DefaultVideoBaseURL is the public MiniMax video-generation API root.
const DefaultVideoBaseURL = "https://api.minimax.io"

// DefaultMusicBaseURL is the public MiniMax music-generation API root.
const DefaultMusicBaseURL = "https://api.minimax.io"

// DefaultSpeechBaseURL is the public MiniMax speech-synthesis API root.
const DefaultSpeechBaseURL = "https://api.minimax.io"

// anthropicCompatSuffix is the path segment DefaultBaseURL carries and the
// media endpoints do not.
const anthropicCompatSuffix = "/anthropic"
