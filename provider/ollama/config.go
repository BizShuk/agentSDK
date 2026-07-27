package ollama

// APIKeyEnvVar configures the optional Bearer token for protected servers
// (LM Studio with auth, vLLM with --api-key, OpenAI). Empty for local
// Ollama, which is key-less by default.
const APIKeyEnvVar = "OPENAI_API_KEY"

// BaseURLEnvVar overrides the default base URL. Local Ollama installs
// keep the default; this lets users point at LAN vLLM / OpenAI.
const BaseURLEnvVar = "OPENAI_BASE_URL"

// DefaultBaseURL points at a local Ollama instance.
const DefaultBaseURL = "http://localhost:11434/v1"
