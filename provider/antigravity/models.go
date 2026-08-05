package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/utils"
)

// Token limits per family. The live catalog endpoint reports none of
// these, and the gateway is undocumented, so they are the published
// family limits confirmed by probing the live endpoint: 65536 output is
// accepted for Gemini and 131072 is rejected.
const (
	GEMINI_CONTEXT_WINDOW = 1048576
	GEMINI_MAX_OUTPUT     = 65536

	CLAUDE_CONTEXT_WINDOW = 200000
	CLAUDE_MAX_OUTPUT     = 64000

	GPT_OSS_CONTEXT_WINDOW = 128000
	GPT_OSS_MAX_OUTPUT     = 16384
)

// catalogEntry is one bundled model's metadata.
//
// Reasoning is absent on purpose: it is derived from the id by
// isThinkingModel, which is the same function that decides whether a
// request takes the SSE path. Hand-writing the flag here let the two
// disagree — the previous table claimed claude-sonnet-4-6 reasons while
// the router sent it down the blocking path.
type catalogEntry struct {
	id     string
	family string
	ctx    int
	max    int
	input  []core.Modality
}

func geminiInput() []core.Modality {
	return []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE, core.MODALITY_AUDIO}
}

func claudeInput() []core.Modality {
	return []core.Modality{core.MODALITY_TEXT, core.MODALITY_IMAGE}
}

// CATALOG carries only models whose limits are actually known.
//
// The live endpoint serves ~24 ids. The rest are deliberately absent
// rather than filled in from family defaults: this gateway is
// undocumented, its tiers do not all share one window, and a guessed
// ContextWindow is worse than no entry — a caller sizing a request
// against a fabricated number gets a 400 it cannot explain.
//
// Everything not listed here is dropped by ListModels, so the opaque IDE
// routing ids (chat_20706, tab_flash_lite_preview) and the untiered
// variants never reach a model picker as rows of zeroes. They remain
// callable by name via --model; they are simply not advertised.
var CATALOG = []catalogEntry{
	// Gemini flash tiers.
	{"gemini-3.6-flash-high", "gemini-flash", GEMINI_CONTEXT_WINDOW, GEMINI_MAX_OUTPUT, geminiInput()},
	{"gemini-3.6-flash-medium", "gemini-flash", GEMINI_CONTEXT_WINDOW, GEMINI_MAX_OUTPUT, geminiInput()},
	{"gemini-3.6-flash-low", "gemini-flash", GEMINI_CONTEXT_WINDOW, GEMINI_MAX_OUTPUT, geminiInput()},
	{"gemini-3.5-flash-low", "gemini-flash", GEMINI_CONTEXT_WINDOW, GEMINI_MAX_OUTPUT, geminiInput()},

	// Gemini pro tiers.
	{"gemini-3.1-pro-high", "gemini-pro", GEMINI_CONTEXT_WINDOW, GEMINI_MAX_OUTPUT, geminiInput()},
	{"gemini-3.1-pro-low", "gemini-pro", GEMINI_CONTEXT_WINDOW, GEMINI_MAX_OUTPUT, geminiInput()},

	// Image-output Gemini, and the default of the ImageGenerator
	// capability. It answers an ordinary chat turn with an inlineData
	// part, so it is listed here rather than in a separate image catalog.
	//
	// The limits follow the Gemini family; MaxTokens is nominal for a
	// model whose output is a picture, and was not probed separately
	// because every probe of this model spends image quota that takes
	// days to reset.
	{"gemini-3.1-flash-image", "gemini-image", GEMINI_CONTEXT_WINDOW, GEMINI_MAX_OUTPUT, geminiInput()},

	// Claude family.
	{"claude-sonnet-4-6", "claude-sonnet", CLAUDE_CONTEXT_WINDOW, CLAUDE_MAX_OUTPUT, claudeInput()},
	{"claude-opus-4-6-thinking", "claude-opus", CLAUDE_CONTEXT_WINDOW, CLAUDE_MAX_OUTPUT, claudeInput()},

	// GPT-OSS family.
	{"gpt-oss-120b-medium", "gpt-oss", GPT_OSS_CONTEXT_WINDOW, GPT_OSS_MAX_OUTPUT, []core.Modality{core.MODALITY_TEXT}},
}

// DefaultCatalog returns the bundled Antigravity model catalog — the
// offline fallback and the source of the metadata the live endpoint does
// not report (family, reasoning flag, token limits).
//
// Both Claude and Gemini families are routed through the same gateway.
// IDs are the strings the gateway accepts on the wire.
func DefaultCatalog() []core.ModelSpec {
	out := make([]core.ModelSpec, 0, len(CATALOG))
	for _, e := range CATALOG {
		out = append(out, core.ModelSpec{
			ID:            e.id,
			Family:        e.family,
			Reasoning:     isThinkingModel(e.id),
			Input:         e.input,
			ContextWindow: e.ctx,
			MaxTokens:     e.max,
		})
	}
	return out
}

// modelsResponse is the fetchAvailableModels body. `models` is keyed by
// model id rather than being a list, which is why this cannot reuse
// provider/utils.DecodeIDList.
type modelsResponse struct {
	Models map[string]struct {
		DisplayName string `json:"displayName"`
		QuotaInfo   *struct {
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
		} `json:"quotaInfo"`
	} `json:"models"`
}

// ListModels implements core.ModelLister against
// /v1internal:fetchAvailableModels.
//
// The endpoint reports which models this account may actually call —
// entitlement varies by subscription tier — so it is a better membership
// source than the bundled catalog. Metadata still comes from
// DefaultCatalog, which the endpoint does not carry.
func (p *Provider) ListModels(ctx context.Context) ([]core.ModelSpec, error) {
	project, err := p.ProjectID(ctx, core.Auth{})
	if err != nil {
		return nil, err
	}
	raw, err := p.post(ctx, PATH_MODELS, map[string]string{"project": project}, core.Auth{}, "")
	if err != nil {
		return nil, fmt.Errorf("antigravity: list models: %w", err)
	}
	var body modelsResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("antigravity: decode model list: %w", err)
	}
	ids := make([]string, 0, len(body.Models))
	for id := range body.Models {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("antigravity: list models: response carried no model ids")
	}
	// The endpoint returns a map, so iteration order is random; a model
	// picker that reshuffles on every call is worse than useless.
	sort.Strings(ids)
	return sized(utils.Merge(ids, DefaultCatalog())), nil
}

// sized drops entries whose token limits are unknown.
//
// utils.Merge keeps a live id the bundled catalog does not recognize,
// carrying the id alone. For most adapters that is the honest answer, but
// this gateway also serves opaque IDE routing ids (chat_20706,
// tab_flash_lite_preview) that are not selectable chat models. They
// surface as rows with zeroes in every column, which reads as broken
// metadata rather than as "internal, not for you".
//
// The cost is real and worth stating: a model Google adds after this
// build will not appear until CATALOG learns its limits. Membership is no
// longer purely live.
func sized(specs []core.ModelSpec) []core.ModelSpec {
	out := make([]core.ModelSpec, 0, len(specs))
	for _, s := range specs {
		if s.ContextWindow == 0 || s.MaxTokens == 0 {
			continue
		}
		out = append(out, s)
	}
	return out
}

// ---------------------------------------------------------------------------
// model family vocabulary
// ---------------------------------------------------------------------------

// The gateway serves several vendors' models behind one surface, and the
// request shape differs by family: thinking config casing, tool-call
// signatures, output-token ceilings. Family is detected from the id
// rather than looked up in the catalog so that a model the gateway added
// after this build still routes correctly.

func isClaudeModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "claude")
}

func isGeminiModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "gemini")
}

// geminiVersion matches the major version in a Gemini id: every Gemini 3
// and later thinks by default.
var geminiVersion = regexp.MustCompile(`gemini-(\d+)`)

// isThinkingModel reports whether the model emits reasoning content.
// Claude advertises it in the id; Gemini does so from version 3 up.
func isThinkingModel(model string) bool {
	lower := strings.ToLower(model)
	if strings.Contains(lower, "thinking") {
		return true
	}
	if !isGeminiModel(lower) {
		return false
	}
	m := geminiVersion.FindStringSubmatch(lower)
	if len(m) < 2 {
		return false
	}
	major, err := strconv.Atoi(m[1])
	return err == nil && major >= 3
}
