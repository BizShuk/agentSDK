package antigravity

import "github.com/bizshuk/agentsdk/core"

// DefaultCatalog returns the bundled Antigravity model catalog.
//
// Models served via the Antigravity gateway as documented by CLIProxyAPI:
//
//	https://help.router-for-me/configuration/provider/antigravity
//
// Both Claude and Gemini families are routed through the gateway. IDs are
// the strings the gateway accepts on the wire; Family is a coarse bucket
// for picker grouping; Reasoning reflects whether the model supports
// extended thinking / chain-of-thought output.
func DefaultCatalog() []core.ModelSpec {
	return []core.ModelSpec{
		// Gemini family (Flash & Pro tiers with thinking support)
		{ID: "gemini-3.6-flash-high", Family: "gemini-flash", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO},
			ContextWindow: 1048576, MaxTokens: 65536},
		{ID: "gemini-3.6-flash-medium", Family: "gemini-flash", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO},
			ContextWindow: 1048576, MaxTokens: 65536},
		{ID: "gemini-3.6-flash-low", Family: "gemini-flash", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO},
			ContextWindow: 1048576, MaxTokens: 65536},
		{ID: "gemini-3.5-flash-high", Family: "gemini-flash", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO},
			ContextWindow: 1048576, MaxTokens: 65536},
		{ID: "gemini-3.5-flash-medium", Family: "gemini-flash", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO},
			ContextWindow: 1048576, MaxTokens: 65536},
		{ID: "gemini-3.5-flash-low", Family: "gemini-flash", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO},
			ContextWindow: 1048576, MaxTokens: 65536},
		{ID: "gemini-3.1-pro-high", Family: "gemini-pro", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO},
			ContextWindow: 1048576, MaxTokens: 65536},
		{ID: "gemini-3.1-pro-low", Family: "gemini-pro", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO},
			ContextWindow: 1048576, MaxTokens: 65536},

		// Claude family
		{ID: "claude-sonnet-4-6", Family: "claude-sonnet", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE},
			ContextWindow: 200000, MaxTokens: 64000},
		{ID: "claude-opus-4-6-thinking", Family: "claude-opus", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE},
			ContextWindow: 200000, MaxTokens: 64000},

		// GPT-OSS family
		{ID: "gpt-oss-120b-medium", Family: "gpt-oss", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT},
			ContextWindow: 128000, MaxTokens: 16384},
	}
}

