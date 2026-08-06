// Static model catalog for xAI Grok. Mirrors provider.ModelSpec so picker
// UIs and budget middleware can plan across providers without reaching
// into provider-specific types.

package grok

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// DefaultCatalog returns the bundled xAI Grok model catalog.
//
// IDs are xAI's published model strings. Family is a coarse bucket used
// for picker grouping. Reasoning reflects whether the model emits visible
// thinking content before its final answer.
//
// This list is intentionally conservative — adding a new model here is
// a user-facing change because picker UIs render it.
func DefaultCatalog() []provider.ModelSpec {
	return []provider.ModelSpec{
		{
			ID:               "grok-4.5",
			Family:           "grok-4",
			Reasoning:        true,
			Capabilities:     []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:    256000,
			MaxTokens:        8192,
		},
		{
			ID:               "grok-2-vision",
			Family:           "grok-2",
			Reasoning:        false,
			Capabilities:     []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:    32768,
			MaxTokens:        8192,
		},
		{
			ID:               DefaultImageModel,
			Family:           "grok-imagine",
			Capabilities:     []provider.Capability{provider.CAPABILITY_IMAGE},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_IMAGE},
		},
	}
}

// ---------------------------------------------------------------------------
// live catalog — GET {base}/models
// ---------------------------------------------------------------------------

// ListModels implements provider.ModelLister against xAI's OpenAI-compatible
// catalog endpoint. The response carries ids only; metadata is merged in
// from DefaultCatalog where the id is recognized.
func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelSpec, error) {
	raw, err := utils.Fetch(ctx, p.client, p.baseURL+"/models", map[string]string{
		"Authorization": p.authHeader(core.Auth{}),
	})
	if err != nil {
		return nil, fmt.Errorf("grok: list models: %w", err)
	}
	ids, err := utils.DecodeIDList(raw)
	if err != nil {
		return nil, fmt.Errorf("grok: %w", err)
	}
	return utils.Merge(ids, DefaultCatalog()), nil
}

// Compile-time: ensure Provider satisfies the optional live-catalog port.
var _ provider.ModelLister = (*Provider)(nil)
