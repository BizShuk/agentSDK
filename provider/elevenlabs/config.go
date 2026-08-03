package elevenlabs

// ElevenLabs accepts long-lived API keys only — no OAuth. Every request
// carries the key in the vendor's own `xi-api-key` header rather than
// `Authorization: Bearer`, which the endpoint does not read.

// APIKeyEnvVar is the standard env var for an ElevenLabs API key.
const APIKeyEnvVar = "ELEVENLABS_API_KEY"

// BaseURLEnvVar lets operators point at a proxy-fronted ElevenLabs endpoint
// without recompiling. Speech and transcription share one host, so there is
// no per-capability override.
const BaseURLEnvVar = "ELEVENLABS_BASE_URL"

// DefaultBaseURL is the public ElevenLabs API root.
const DefaultBaseURL = "https://api.elevenlabs.io"

// DefaultSpeechModel is the low-latency text-to-speech model used when a
// request names none.
const DefaultSpeechModel = "eleven_flash_v2_5"

// DefaultTranscribeModel is the speech-to-text model used when a request
// names none.
const DefaultTranscribeModel = "scribe_v1"

// DefaultVoiceID is the stock "Rachel" voice, used when a request names no
// voice. Voice ids are account-scoped identifiers, not display names.
const DefaultVoiceID = "21m00Tcm4TlvDq8ikWAM"

// DefaultSpeechOutputFormat labels the audio ElevenLabs returns when the
// request sends no explicit output_format.
const DefaultSpeechOutputFormat = "mp3_44100_128"

// APIKeyHeader is the vendor's credential header name.
const APIKeyHeader = "xi-api-key"
