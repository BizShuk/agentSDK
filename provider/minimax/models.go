package minimax

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/provider"
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
func DefaultCatalog() []provider.ModelSpec {
	return []provider.ModelSpec{
		// MiniMax-M3 family — current flagship, supports reasoning.
		{ID: "MiniMax-M3", Family: "MiniMax-M3", Reasoning: true,
			Capabilities:     []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:    256000, MaxTokens: 16384},
		// minimax-M2 — previous reasoning model, retained for explicit selection.
		{ID: "minimax-M2", Family: "minimax-M2", Reasoning: true,
			Capabilities:     []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:    256000, MaxTokens: 16384},
		// MiniMax-Text-01 — base text model, no reasoning.
		{ID: "MiniMax-Text-01", Family: "minimax-Text", Reasoning: false,
			Capabilities:     []provider.Capability{provider.CAPABILITY_CHAT},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:    128000, MaxTokens: 8192},

		// image-* — image_generation models reached through
		// provider.ImageGenerator, not through Generate. Token bounds do
		// not apply; the prompt is bounded by characters.
		{ID: "image-01", Family: "image-01",
			Capabilities:     []provider.Capability{provider.CAPABILITY_IMAGE},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_IMAGE}},
		{ID: "image-01-live", Family: "image-01",
			Capabilities:     []provider.Capability{provider.CAPABILITY_IMAGE},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_IMAGE}},

		// video — video_generation models reached through
		// provider.VideoGenerator. MiniMax-H3 (Hailuo 03) is the current
		// flagship: t2v / i2v / first-last frame / multimodal reference,
		// 768P-2K, 4-15s at 24 fps. The Hailuo-2.x and S2V entries are the
		// legacy models the adapter's per-mode defaults still name.
		{ID: "MiniMax-H3", Family: "MiniMax-H3",
			Capabilities:     []provider.Capability{provider.CAPABILITY_VIDEO},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_VIDEO}},
		{ID: "MiniMax-Hailuo-2.3", Family: "MiniMax-Hailuo",
			Capabilities:     []provider.Capability{provider.CAPABILITY_VIDEO},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_VIDEO}},
		{ID: "MiniMax-Hailuo-02", Family: "MiniMax-Hailuo",
			Capabilities:     []provider.Capability{provider.CAPABILITY_VIDEO},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_VIDEO}},
		{ID: "S2V-01", Family: "S2V",
			Capabilities:     []provider.Capability{provider.CAPABILITY_VIDEO},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			OutputModalities: []provider.Modality{provider.MODALITY_VIDEO}},

		// music — music generation models reached through
		// provider.MusicGenerator. music-cover conditions on reference
		// audio; the text models are bounded by prompt/lyrics characters.
		{ID: "music-3.0", Family: "music-3.0",
			Capabilities:     []provider.Capability{provider.CAPABILITY_MUSIC},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "music-2.6", Family: "music-2.6",
			Capabilities:     []provider.Capability{provider.CAPABILITY_MUSIC},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "music-cover", Family: "music-cover",
			Capabilities:     []provider.Capability{provider.CAPABILITY_MUSIC},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_AUDIO},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},

		// speech-* — t2a_v2 voices reached through provider.SpeechGenerator,
		// not through Generate. ContextWindow / MaxTokens stay zero because
		// t2a_v2 bounds a request by characters of input text, not tokens.
		// speech-2.8 is the current generation (40 languages, 7 emotions);
		// older families remain for explicit selection.
		{ID: "speech-2.8-hd", Family: "speech-2.8",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "speech-2.8-turbo", Family: "speech-2.8",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "speech-2.5-hd-preview", Family: "speech-2.5",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "speech-2.5-turbo-preview", Family: "speech-2.5",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "speech-02-hd", Family: "speech-02",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "speech-02-turbo", Family: "speech-02",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "speech-01-hd", Family: "speech-01",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
		{ID: "speech-01-turbo", Family: "speech-01",
			Capabilities:     []provider.Capability{provider.CAPABILITY_SPEECH},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_AUDIO}},
	}
}

// ---------------------------------------------------------------------------
// live catalog — GET {base}/v1/models
// ---------------------------------------------------------------------------

// ListModels implements provider.ModelLister. minimax mirrors Anthropic's
// catalog shape on its Anthropic-compat surface, so the response is an
// id list; context windows and reasoning flags are merged in from
// DefaultCatalog for ids we ship metadata for.
func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelSpec, error) {
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
var _ provider.ModelLister = (*Provider)(nil)
