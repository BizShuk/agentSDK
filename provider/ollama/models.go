package ollama

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// DefaultCatalog lists common Ollama / OpenAI-compatible model ids.
//
// Most users override this via WithModel or a viper-loaded config; this
// is the fallback set the picker UI sees when nothing else is supplied.
//
// Reasoning = true flags the "thinking" family (gpt-oss / deepseek-r1);
// picker UIs can use that to expose reasoning budget controls.
func DefaultCatalog() []core.ModelSpec {
	return []core.ModelSpec{
		{ID: "llama3.2", Family: "llama", Reasoning: false,
			Input:         []core.Modality{core.MODALITY_TEXT},
			ContextWindow: 128000, MaxTokens: 8192},
		{ID: "qwen2.5", Family: "qwen", Reasoning: false,
			Input:         []core.Modality{core.MODALITY_TEXT},
			ContextWindow: 128000, MaxTokens: 8192},
		{ID: "mistral", Family: "mistral", Reasoning: false,
			Input:         []core.Modality{core.MODALITY_TEXT},
			ContextWindow: 32000, MaxTokens: 8192},
		{ID: "gpt-oss:20b", Family: "gpt-oss", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT},
			ContextWindow: 128000, MaxTokens: 8192},
		{ID: "deepseek-r1", Family: "deepseek", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT},
			ContextWindow: 128000, MaxTokens: 8192},
		{ID: "llava", Family: "llava", Reasoning: false,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE},
			ContextWindow: 32000, MaxTokens: 4096},
	}
}

// ---------------------------------------------------------------------------
// live catalog — GET {base}/models
// ---------------------------------------------------------------------------

// ListModels implements core.ModelLister. This adapter fronts whatever
// OpenAI-compatible server the base URL points at, so the live call is
// especially valuable here: the models a local Ollama has actually pulled
// are a per-machine fact no bundled catalog can know.
//
// The Authorization header is omitted entirely when no key is configured —
// local Ollama is keyless and rejects nothing, but LM Studio and vLLM
// deployments behind auth need the Bearer.
func (p *Provider) ListModels(ctx context.Context) ([]core.ModelSpec, error) {
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
var _ core.ModelLister = (*Provider)(nil)
