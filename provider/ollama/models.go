package ollama

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// DefaultCatalog lists common Ollama / OpenAI-compatible model ids.
//
// Most users override this through provider.Options.Model; this is the
// fallback set the picker UI sees when nothing else is supplied.
//
// Reasoning = true flags the "thinking" family (gpt-oss / deepseek-r1);
// picker UIs can use that to expose reasoning budget controls.
func DefaultCatalog() []provider.ModelSpec {
	return []provider.ModelSpec{
		{
			ID: "qwen2.5vl:3b", Family: "qwen", Reasoning: false,
			Capabilities:     []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:    128000, MaxTokens: 8192,
		},
		{
			// This model requires a nonstandard tool endpoint the chat adapter
			// does not implement, so catalog identity is retained without an
			// executable capability.
			ID: "z-uo/qwen2.5vl_tools:7b", Family: "qwen", Reasoning: false,
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:    128000, MaxTokens: 8192,
		},
		{
			ID: "gemma4:e2b", Family: "gemma", Reasoning: false,
			Capabilities:     []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:    128000, MaxTokens: 8192,
		},
		{
			// Embedding has no typed provider surface in this SDK.
			ID: "bge-m3:latest", Family: "bge", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   8192, MaxTokens: 2048,
		},
	}
}

// ---------------------------------------------------------------------------
// live catalog — GET {base}/models
// ---------------------------------------------------------------------------

// ListModels implements provider.ModelLister. This adapter fronts whatever
// OpenAI-compatible server the base URL points at, so the live call is
// especially valuable here: the models a local Ollama has actually pulled
// are a per-machine fact no bundled catalog can know.
//
// The Authorization header is omitted entirely when no key is configured —
// local Ollama is keyless and rejects nothing, but LM Studio and vLLM
// deployments behind auth need the Bearer.
func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelSpec, error) {
	headers := map[string]string{}
	if p.auth.Token() != "" {
		headers["Authorization"] = "Bearer " + p.auth.Token()
	}
	raw, err := utils.Fetch(ctx, p.client, p.baseURL+"/models", headers)
	if err != nil {
		return nil, fmt.Errorf("ollama: list models: %w", err)
	}
	ids, err := utils.DecodeIDList(raw)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	return utils.Merge(ids, DefaultCatalog()), nil
}

// Compile-time: ensure Provider satisfies the optional live-catalog port.
var _ provider.ModelLister = (*Provider)(nil)
