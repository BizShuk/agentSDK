package google

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// DefaultCatalog lists the Google Generative AI models the proxy
// serves by default. Snapshot taken from the live
// {v1beta}/models?pageSize=1000 endpoint on 2026-07-20; the list
// below carries the 39 entries that report generateContent in
// supportedGenerationMethods. Embedding and Imagen models are
// intentionally omitted — the live lister filters them out anyway
// (embedContent-only or non-chat surfaces), so including them here
// would only add noise to picker UIs.
//
// Families present in this snapshot:
//
//   - gemini-*    : Gemini chat + TTS + image. The "-flash" suffix
//     is a coarse bucket for the Flash tier; Reasoning mirrors
//     Google's "thinking" flag where applicable (none of the
//     snapshot's 39 entries report it as true).
//   - gemma-*     : Gemma open chat models — text only.
//   - nano-*      : Nano-banana image generation/editing.
//   - lyria-*     : Lyria music generation.
//   - antigravity : Antigravity agentic preview.
//   - deep-*      : Deep-research tool-using models.
//
// The live lister fills token limits from the API response and
// borrows family / reasoning / input modality from this catalog
// when the id is recognized; ids that ship after the binary was
// built come back with the id alone rather than being hidden.
func DefaultCatalog() []provider.ModelSpec {
	specs := []provider.ModelSpec{
		// -----------------------------------------------------------------
		// Gemini 2.x — multimodal chat
		// -----------------------------------------------------------------
		{
			ID: "gemini-2.5-flash", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-2.5-pro", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-2.5-flash-lite", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-2.0-flash", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 8192,
		},
		{
			ID: "gemini-2.0-flash-001", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 8192,
		},
		{
			ID: "gemini-2.0-flash-lite", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 8192,
		},
		{
			ID: "gemini-2.0-flash-lite-001", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 8192,
		},

		// -----------------------------------------------------------------
		// Gemini 2.x — TTS (text → speech)
		// -----------------------------------------------------------------
		{
			ID: "gemini-2.5-flash-preview-tts", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   8192, MaxTokens: 16384,
		},
		{
			ID: "gemini-2.5-pro-preview-tts", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   8192, MaxTokens: 16384,
		},

		// -----------------------------------------------------------------
		// Gemini "latest" aliases — track the GA pointer
		// -----------------------------------------------------------------
		{
			ID: "gemini-flash-latest", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-flash-lite-latest", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-pro-latest", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},

		// -----------------------------------------------------------------
		// Gemini 2.5 image
		// -----------------------------------------------------------------
		{
			ID: "gemini-2.5-flash-image", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   32768, MaxTokens: 32768,
		},

		// -----------------------------------------------------------------
		// Gemini 3.x — multimodal chat
		// -----------------------------------------------------------------
		{
			ID: "gemini-3-pro-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-3-flash-preview", Family: "gemini-flash", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-3.1-pro-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-3.1-pro-preview-customtools", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-3.1-flash-lite-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-3.1-flash-lite", Family: "gemini-flash", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-3.5-flash", Family: "gemini-flash", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},

		// -----------------------------------------------------------------
		// Gemini 3.x image generation/editing
		// -----------------------------------------------------------------
		{
			ID: "gemini-3-pro-image-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   131072, MaxTokens: 32768,
		},
		{
			ID: "gemini-3-pro-image", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   131072, MaxTokens: 32768,
		},
		{
			ID: "gemini-3.1-flash-image-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   65536, MaxTokens: 65536,
		},
		{
			ID: "gemini-3.1-flash-image", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   65536, MaxTokens: 65536,
		},
		{
			ID: "gemini-3.1-flash-lite-image", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   65536, MaxTokens: 65536,
		},

		// -----------------------------------------------------------------
		// Gemini 3.x TTS
		// -----------------------------------------------------------------
		{
			ID: "gemini-3.1-flash-tts-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   8192, MaxTokens: 16384,
		},

		// -----------------------------------------------------------------
		// Gemini omni / robotics / computer-use (vision-language)
		// -----------------------------------------------------------------
		{
			ID: "gemini-omni-flash-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   131072, MaxTokens: 65536,
		},
		{
			ID: "gemini-robotics-er-1.5-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "gemini-robotics-er-1.6-preview", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   131072, MaxTokens: 65536,
		},
		{
			ID: "gemini-2.5-computer-use-preview-10-2025", Family: "gemini", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   131072, MaxTokens: 65536,
		},

		// -----------------------------------------------------------------
		// Lyria — music generation (text prompt → audio)
		// -----------------------------------------------------------------
		{
			ID: "lyria-3-clip-preview", Family: "lyria", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   1048576, MaxTokens: 65536,
		},
		{
			ID: "lyria-3-pro-preview", Family: "lyria", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   1048576, MaxTokens: 65536,
		},

		// -----------------------------------------------------------------
		// Gemma 4 — open chat (text only)
		// -----------------------------------------------------------------
		{
			ID: "gemma-4-31b-it", Family: "gemma", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   262144, MaxTokens: 32768,
		},
		{
			ID: "gemma-4-26b-a4b-it", Family: "gemma", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   262144, MaxTokens: 32768,
		},

		// -----------------------------------------------------------------
		// Nano-banana — image generation/editing
		// -----------------------------------------------------------------
		{
			ID: "nano-banana-pro-preview", Family: "nano", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   131072, MaxTokens: 32768,
		},

		// -----------------------------------------------------------------
		// Antigravity — agentic preview
		// -----------------------------------------------------------------
		{
			ID: "antigravity-preview-05-2026", Family: "antigravity", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE},
			ContextWindow:   131072, MaxTokens: 65536,
		},

		// -----------------------------------------------------------------
		// Deep research — tool-using research models
		// -----------------------------------------------------------------
		{
			ID: "deep-research-max-preview-04-2026", Family: "deep", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   131072, MaxTokens: 65536,
		},
		{
			ID: "deep-research-preview-04-2026", Family: "deep", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   131072, MaxTokens: 65536,
		},
		{
			ID: "deep-research-pro-preview-12-2025", Family: "deep", Reasoning: false,
			InputModalities: []provider.Modality{provider.MODALITY_TEXT},
			ContextWindow:   131072, MaxTokens: 65536,
		},
	}
	for i := range specs {
		annotateCatalogModel(&specs[i])
	}
	return append(specs,
		provider.ModelSpec{
			ID:               DefaultLiveModel,
			Family:           "gemini-live",
			Capabilities:     []provider.Capability{provider.CAPABILITY_LIVE},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_AUDIO},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_AUDIO},
		},
		provider.ModelSpec{
			ID:               DefaultTranslateModel,
			Family:           "gemini-live-translate",
			Capabilities:     []provider.Capability{provider.CAPABILITY_TRANSLATE},
			InputModalities:  []provider.Modality{provider.MODALITY_TEXT},
			OutputModalities: []provider.Modality{provider.MODALITY_TEXT},
		},
	)
}

// annotateCatalogModel assigns operation metadata by exact catalog ID. The
// lists are explicit catalog data, not model-name inference.
func annotateCatalogModel(spec *provider.ModelSpec) {
	switch spec.ID {
	case
		"gemini-2.5-flash",
		"gemini-2.5-pro",
		"gemini-2.5-flash-lite",
		"gemini-2.0-flash",
		"gemini-2.0-flash-001",
		"gemini-2.0-flash-lite",
		"gemini-2.0-flash-lite-001",
		"gemini-flash-latest",
		"gemini-flash-lite-latest",
		"gemini-pro-latest",
		"gemini-3-pro-preview",
		"gemini-3-flash-preview",
		"gemini-3.1-pro-preview",
		"gemini-3.1-pro-preview-customtools",
		"gemini-3.1-flash-lite-preview",
		"gemini-3.1-flash-lite",
		"gemini-3.5-flash",
		"gemini-omni-flash-preview",
		"gemini-robotics-er-1.5-preview",
		"gemini-robotics-er-1.6-preview",
		"gemini-2.5-computer-use-preview-10-2025",
		"gemma-4-31b-it",
		"gemma-4-26b-a4b-it",
		"antigravity-preview-05-2026",
		"deep-research-max-preview-04-2026",
		"deep-research-preview-04-2026",
		"deep-research-pro-preview-12-2025":
		spec.Capabilities = []provider.Capability{provider.CAPABILITY_CHAT}
		spec.OutputModalities = []provider.Modality{provider.MODALITY_TEXT}
	case
		"gemini-2.5-flash-image",
		"gemini-3-pro-image-preview",
		"gemini-3-pro-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-lite-image",
		"nano-banana-pro-preview":
		spec.Capabilities = []provider.Capability{provider.CAPABILITY_IMAGE}
		spec.OutputModalities = []provider.Modality{provider.MODALITY_IMAGE}
	case
		"gemini-2.5-flash-preview-tts",
		"gemini-2.5-pro-preview-tts",
		"gemini-3.1-flash-tts-preview",
		"lyria-3-clip-preview",
		"lyria-3-pro-preview":
		spec.OutputModalities = []provider.Modality{provider.MODALITY_AUDIO}
	}
}

// ---------------------------------------------------------------------------
// live catalog — GET {v1beta}/models
// ---------------------------------------------------------------------------

// MODELS_PAGE_SIZE asks Google for the whole catalog in one round trip.
// 1000 is the documented maximum; the catalog is ~50 entries, so the
// pagination loop below normally runs exactly once.
const MODELS_PAGE_SIZE = 1000

// MAX_MODEL_PAGES bounds the pagination loop. A server that keeps handing
// back a nextPageToken forever would otherwise spin indefinitely.
const MAX_MODEL_PAGES = 10

// GENERATE_CONTENT_METHOD is the capability that marks a catalog entry as
// usable for chat. Google lists embedding and image models in the same
// response, distinguished only by this field.
const GENERATE_CONTENT_METHOD = "generateContent"

// nativeModel is one entry of Google's native (non-OpenAI-compat) catalog
// response. The OpenAI-compat surface at /v1beta/openai/models reports ids
// only, so we query the native endpoint instead — it carries the token
// limits, which are exactly the metadata the bundled catalog goes stale on.
type nativeModel struct {
	Name                       string   `json:"name"` // "models/gemini-3-flash-preview"
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

// nativeListResponse is the paginated envelope wrapping nativeModel.
type nativeListResponse struct {
	Models        []nativeModel `json:"models"`
	NextPageToken string        `json:"nextPageToken"`
}

// ListModels implements provider.ModelLister. It queries Google's live model
// catalog and returns only entries that support generateContent — the
// embedding and Imagen entries Google also lists cannot serve a chat
// request, so surfacing them in a model picker would offer choices that
// fail at call time.
//
// Token limits come from the API (authoritative, and the reason we use the
// native endpoint over the OpenAI-compat one); family and reasoning flags
// come from DefaultCatalog where the id is recognized, since the catalog
// endpoint does not report them.
func (p *Provider) ListModels(ctx context.Context) ([]provider.ModelSpec, error) {
	static := indexCatalog()
	var out []provider.ModelSpec
	token := ""

	for range MAX_MODEL_PAGES {
		url := fmt.Sprintf("%s/models?pageSize=%d", p.nativeBaseURL(), MODELS_PAGE_SIZE)
		if token != "" {
			url += "&pageToken=" + token
		}
		raw, err := utils.Fetch(ctx, p.client, url, map[string]string{
			"x-goog-api-key": p.auth.Token(),
		})
		if err != nil {
			return nil, fmt.Errorf("google: list models: %w", err)
		}
		var body nativeListResponse
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("google: decode model list: %w", err)
		}
		for _, m := range body.Models {
			if !supportsGenerateContent(m) {
				continue
			}
			out = append(out, m.toSpec(static))
		}
		if body.NextPageToken == "" {
			break
		}
		token = body.NextPageToken
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("google: list models: no generateContent-capable models returned")
	}
	return out, nil
}

// nativeBaseURL converts the configured OpenAI-compat base URL into the
// native one by dropping the trailing "/openai" segment. A caller who
// already supplied a native endpoint in ResolvedConfig is left untouched.
func (p *Provider) nativeBaseURL() string {
	return strings.TrimSuffix(p.baseURL, "/openai")
}

// supportsGenerateContent reports whether the entry can serve chat.
func supportsGenerateContent(m nativeModel) bool {
	return slices.Contains(m.SupportedGenerationMethods, GENERATE_CONTENT_METHOD)
}

// toSpec folds one live catalog entry into a provider.ModelSpec, preferring
// the API's token limits and borrowing family / reasoning / modality from
// the bundled catalog when the id is one we ship metadata for.
func (m nativeModel) toSpec(static map[string]provider.ModelSpec) provider.ModelSpec {
	id := strings.TrimPrefix(m.Name, "models/")
	spec := provider.ModelSpec{
		ID:            id,
		ContextWindow: m.InputTokenLimit,
		MaxTokens:     m.OutputTokenLimit,
	}
	if known, ok := static[id]; ok {
		spec.Family = known.Family
		spec.Reasoning = known.Reasoning
		spec.Capabilities = slices.Clone(known.Capabilities)
		spec.InputModalities = known.InputModalities
		spec.OutputModalities = known.OutputModalities
		return spec
	}
	// The endpoint itself established generateContent support. It does not
	// report richer modality metadata, so unknown entries remain text chat.
	spec.Capabilities = []provider.Capability{provider.CAPABILITY_CHAT}
	spec.InputModalities = []provider.Modality{provider.MODALITY_TEXT}
	spec.OutputModalities = []provider.Modality{provider.MODALITY_TEXT}
	return spec
}

// indexCatalog keys DefaultCatalog by model id for O(1) metadata lookup.
func indexCatalog() map[string]provider.ModelSpec {
	catalog := DefaultCatalog()
	out := make(map[string]provider.ModelSpec, len(catalog))
	for _, s := range catalog {
		out[s.ID] = s
	}
	return out
}

// Compile-time: ensure Provider satisfies the optional live-catalog port.
var _ provider.ModelLister = (*Provider)(nil)
