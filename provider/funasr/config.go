// Package funasr adapts a self-hosted FunASR OpenAI-compatible HTTP server
// (examples/openai_api/server.py upstream, or the docker deployment in
// ~/projects/platform/inf) to provider.Transcriber.
//
// The server is local-first and keyless by default: credentials exist only
// for a gateway-fronted deployment. Base URL and key are overridable through
// the registry env metadata, so a viper-backed Options.LookupEnv (the CLI
// path) lets config files participate without this package importing viper.
package funasr

// APIKeyEnvVar is consulted for a gateway-fronted deployment; the stock
// server ignores credentials entirely.
const APIKeyEnvVar = "FUNASR_API_KEY"

// BaseURLEnvVar overrides the server address without recompiling.
const BaseURLEnvVar = "FUNASR_BASE_URL"

// DefaultBaseURL is the local server the docker deployment publishes.
const DefaultBaseURL = "http://localhost:8000"

// DefaultTranscribeModel is used when neither config nor request names one.
// SenseVoice is the CPU-friendly default the upstream server also preloads.
const DefaultTranscribeModel = "sensevoice"
