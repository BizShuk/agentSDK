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
