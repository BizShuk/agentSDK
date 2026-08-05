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

// DefaultCatalog returns the bundled Antigravity model catalog — the
// offline fallback and the source of the metadata the live endpoint does
// not report (family, reasoning flag, context window).
//
// Both Claude and Gemini families are routed through the same gateway.
// IDs are the strings the gateway accepts on the wire.
//
// This is a subset by design. The gateway serves ~24 ids to a live
// account, including internal ones (chat_20706, tab_flash_lite_preview)
// and tiers whose context limits are not published anywhere. ListModels
// is the authority on membership; entries here exist to supply the
// metadata that endpoint does not report, so an id is listed only when
// its family and limits are actually known.
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
		// The 3.5 tier ships -low and -extra-low only; there is no
		// -medium or -high, verified against a live fetchAvailableModels.
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
	return utils.Merge(ids, DefaultCatalog()), nil
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
