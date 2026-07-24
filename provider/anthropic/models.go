package anthropic

// Static model catalog the SDK ships by default. Mirrors core.ModelSpec
// so picker UIs and budget middleware can plan across providers without
// reaching into Anthropic-specific types.

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// DefaultCatalog returns the bundled Anthropic model catalog.
//
// IDs are Anthropic's published model strings. Family is a coarse bucket
// used for picker grouping; Reasoning reflects whether the model is in
// the "extended thinking" family.
//
// This list is intentionally conservative — adding a new model here is a
// user-facing change because picker UIs render it. Add new models in a
// follow-up after they ship a stable API.
func DefaultCatalog() []core.ModelSpec {
	return []core.ModelSpec{
		{ID: "claude-opus-4-8", Family: "claude-opus", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE},
			ContextWindow: 200000, MaxTokens: 32000},
		{ID: "claude-sonnet-5", Family: "claude-sonnet", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE},
			ContextWindow: 200000, MaxTokens: 8192},
		{ID: "claude-haiku-4-5-20251001", Family: "claude-haiku", Reasoning: false,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE},
			ContextWindow: 200000, MaxTokens: 8192},
	}
}

// ---------------------------------------------------------------------------
// live catalog — GET {base}/v1/models
// ---------------------------------------------------------------------------

// ListModels implements core.ModelLister against Anthropic's catalog
// endpoint. The endpoint reports ids and display names only, so context
// windows and reasoning flags are merged in from DefaultCatalog; ids
// Anthropic has published since this binary was built come back with the
// id alone rather than being hidden.
//
// The same call works against an Anthropic-compatible gateway, since the
// URL is derived from the configured endpoint rather than hard-coded.
func (p *Provider) ListModels(ctx context.Context) ([]core.ModelSpec, error) {
	raw, err := utils.Fetch(ctx, p.httpDoer, p.modelsEndpoint(), p.catalogHeaders())
	if err != nil {
		return nil, fmt.Errorf("anthropic: list models: %w", err)
	}
	ids, err := utils.DecodeIDList(raw)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	return utils.Merge(ids, DefaultCatalog()), nil
}

// modelsEndpoint swaps the /messages path for /models on whatever base the
// provider was constructed with, so a gateway override carries over.
func (p *Provider) modelsEndpoint() string {
	return strings.TrimSuffix(p.endpoint, "/messages") + "/models"
}

// catalogHeaders assembles the auth + version headers the catalog endpoint
// requires. Mirrors applyAuthHeaders, but as a map for utils.Fetch.
func (p *Provider) catalogHeaders() map[string]string {
	h := map[string]string{"anthropic-version": p.apiVer}
	if p.auth.Bearer != "" {
		h["Authorization"] = "Bearer " + p.auth.Bearer
	} else if p.auth.APIKey != "" {
		h["x-api-key"] = p.auth.APIKey
	}
	maps.Copy(h, p.auth.Headers)
	return h
}

// Compile-time: ensure Provider satisfies the optional live-catalog port.
var _ core.ModelLister = (*Provider)(nil)
