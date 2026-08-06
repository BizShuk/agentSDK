package google

// APIKeyEnvVar configures the required Bearer token for Google AI Studio.
// Empty API key fails construction in New(); there is no keyless path.
const APIKeyEnvVar = "GOOGLE_API_KEY"

// BaseURLEnvVar overrides the default base URL. Most users keep the
// default; this exists for proxying via Vertex AI or local mirrors.
const BaseURLEnvVar = "GOOGLE_BASE_URL"

// DefaultBaseURL points at Google Generative AI's OpenAI-compatible
// endpoint at /v1beta/openai.
const DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"

// LiveBaseURLEnvVar overrides the realtime websocket endpoint used by the
// Live API session and translation surfaces.
const LiveBaseURLEnvVar = "GOOGLE_LIVE_BASE_URL"

// DefaultLiveBaseURL is the Gemini Live API BidiGenerateContent websocket.
const DefaultLiveBaseURL = "wss://generativelanguage.googleapis.com/ws/" +
	"google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent"

// DefaultLiveModel is the recommended Live API dialogue model: low-latency
// realtime conversation with native audio output and thinkingLevel control.
const DefaultLiveModel = "gemini-3.1-flash-live-preview"

// DefaultTranslateModel is the realtime streaming translation model served
// over the same Live API socket.
const DefaultTranslateModel = "gemini-3.5-live-translate-preview"
