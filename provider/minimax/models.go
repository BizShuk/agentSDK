package minimax

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// DefaultCatalog returns the bundled minimax model catalog.
//
// Model ids mirror what the proxy/svc/upstream/profile.go advertises
// under the "minimax" provider ("MiniMax-Text-01" and the "minimax-*"
// prefix). Reasoning reflects whether the model is in the extended
// thinking family; ContextWindow / MaxTokens are best-effort estimates
// from the public docs — callers needing exact limits should consult
// the API docs at https://docs.minimax.io directly.
//
// This list is intentionally conservative — adding a new model here is
// a user-facing change because picker UIs render it. Add new models in
// a follow-up after they ship a stable API.
func DefaultCatalog() []core.ModelSpec {
	return []core.ModelSpec{
		// MiniMax-M3 family — current flagship, supports reasoning.
		{ID: "MiniMax-M3", Family: "MiniMax-M3", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE},
			ContextWindow: 256000, MaxTokens: 16384},
		// minimax-M2 — previous reasoning model, retained for explicit selection.
		{ID: "minimax-M2", Family: "minimax-M2", Reasoning: true,
			Input:         []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE},
			ContextWindow: 256000, MaxTokens: 16384},
		// MiniMax-Text-01 — base text model, no reasoning.
		{ID: "MiniMax-Text-01", Family: "minimax-Text", Reasoning: false,
			Input:         []core.Modality{core.MODALITY_TEXT},
			ContextWindow: 128000, MaxTokens: 8192},

		// speech-* — t2a_v2 voices reached through provider.SpeechGenerator,
		// not through Generate. ContextWindow / MaxTokens stay zero because
		// t2a_v2 bounds a request by characters of input text, not tokens.
		{ID: "speech-2.5-hd-preview", Family: "speech-2.5",
			Input: []core.Modality{core.MODALITY_TEXT}},
		{ID: "speech-2.5-turbo-preview", Family: "speech-2.5",
			Input: []core.Modality{core.MODALITY_TEXT}},
		{ID: "speech-02-hd", Family: "speech-02",
			Input: []core.Modality{core.MODALITY_TEXT}},
		{ID: "speech-02-turbo", Family: "speech-02",
			Input: []core.Modality{core.MODALITY_TEXT}},
		{ID: "speech-01-hd", Family: "speech-01",
			Input: []core.Modality{core.MODALITY_TEXT}},
		{ID: "speech-01-turbo", Family: "speech-01",
			Input: []core.Modality{core.MODALITY_TEXT}},
	}
}

// ---------------------------------------------------------------------------
// live catalog — GET {base}/v1/models
// ---------------------------------------------------------------------------

// ListModels implements core.ModelLister. minimax mirrors Anthropic's
// catalog shape on its Anthropic-compat surface, so the response is an
// id list; context windows and reasoning flags are merged in from
// DefaultCatalog for ids we ship metadata for.
func (p *Provider) ListModels(ctx context.Context) ([]core.ModelSpec, error) {
	raw, err := utils.Fetch(ctx, p.client, p.baseURL+"/v1/models", map[string]string{
		"X-Api-Key": p.auth.Token(),
	})
	if err != nil {
		return nil, fmt.Errorf("minimax: list models: %w", err)
	}
	ids, err := utils.DecodeIDList(raw)
	if err != nil {
		return nil, fmt.Errorf("minimax: %w", err)
	}
	return utils.Merge(ids, DefaultCatalog()), nil
}

// Compile-time: ensure Provider satisfies the optional live-catalog port.
var _ core.ModelLister = (*Provider)(nil)
