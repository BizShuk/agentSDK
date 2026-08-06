package codex

import "github.com/bizshuk/agentsdk/provider"

// DefaultCatalog returns the bundled Codex model catalog. IDs are
// the upstream strings published by OpenAI for ChatGPT-Plus/Pro
// subscribers.
//
// Family is a coarse bucket ("gpt-5") for picker grouping; the
// Lite flag in IsLiteModel controls the parallel_tool_calls
// toggle — see IsLiteModel below.
//
// Add new models in a follow-up after they ship a stable API.
func DefaultCatalog() []provider.ModelSpec {
	return []provider.ModelSpec{
		{
			ID: "gpt-5.5", Family: "gpt-5", Reasoning: true,
			Capabilities:    []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities: []provider.Modality{provider.MODALITY_TEXT}, ContextWindow: 200000, MaxTokens: 16384,
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
		},
		{
			ID: "gpt-5-mini", Family: "gpt-5", Reasoning: false,
			Capabilities:    []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities: []provider.Modality{provider.MODALITY_TEXT}, ContextWindow: 200000, MaxTokens: 16384,
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
		},
		{
			ID: "gpt-5.6-sol", Family: "gpt-5.6", Reasoning: true,
			Capabilities:    []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities: []provider.Modality{provider.MODALITY_TEXT}, ContextWindow: 200000, MaxTokens: 16384,
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
		},
		{
			ID: "gpt-5.6-terra", Family: "gpt-5.6", Reasoning: true,
			Capabilities:    []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities: []provider.Modality{provider.MODALITY_TEXT}, ContextWindow: 200000, MaxTokens: 16384,
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
		},
		{
			ID: "gpt-5.6-luna", Family: "gpt-5.6", Reasoning: true,
			Capabilities:    []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities: []provider.Modality{provider.MODALITY_TEXT}, ContextWindow: 200000, MaxTokens: 16384,
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
		},
		{
			ID:               DefaultLiveModel,
			Family:           "gpt-realtime",
			Capabilities:     []provider.Capability{provider.CAPABILITY_LIVE},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_AUDIO},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_AUDIO},
		},
	}
}

// IsLiteModel reports whether the given model ID forces
// parallel_tool_calls=false in the Codex wire request.
//
// The "lite" tier was introduced with the 5.6 model family — these
// variants are optimized for low-latency reply but reject concurrent
// tool dispatch (which Codex interprets as multiple "go do thing
// A AND thing B" intents and rejects).
//
// Add new lite suffixes here when OpenAI introduces them — the
// pattern is `<family>.6` and `<family>.6-sol`.
func IsLiteModel(model string) bool {
	switch model {
	case "gpt-5.6", "gpt-5.6-sol":
		return true
	}
	return false
}
