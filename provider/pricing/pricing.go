// Package pricing owns the versioned public-rate snapshot and deterministic
// conversion from provider usage to USD cost.
package pricing

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
	"unicode"

	"github.com/bizshuk/agentsdk/core"
)

// OPENROUTER_MODELS_URL is the canonical public pricing manifest.
const OPENROUTER_MODELS_URL = "https://openrouter.ai/api/v1/models?output_modalities=all"

// Rate is OpenRouter's USD price for one billing unit.
type Rate struct {
	Prompt         string         `json:"prompt,omitempty"`
	Completion     string         `json:"completion,omitempty"`
	WebSearch      string         `json:"web_search,omitempty"`
	InputCacheRead string         `json:"input_cache_read,omitempty"`
	Overrides      []RateOverride `json:"overrides,omitempty"`
}

// RateOverride replaces base token rates at a prompt-size threshold.
type RateOverride struct {
	MinPromptTokens int    `json:"min_prompt_tokens"`
	Prompt          string `json:"prompt,omitempty"`
	Completion      string `json:"completion,omitempty"`
	WebSearch       string `json:"web_search,omitempty"`
	InputCacheRead  string `json:"input_cache_read,omitempty"`
}

// Snapshot is the checked-in subset of OpenRouter model pricing consumed by
// AgentSDK. Keys are OpenRouter model IDs.
type Snapshot struct {
	Source      string          `json:"source"`
	PricingAsOf string          `json:"pricing_as_of"`
	Models      map[string]Rate `json:"models"`
}

type openRouterManifest struct {
	Data []struct {
		ID      string `json:"id"`
		Pricing Rate   `json:"pricing"`
	} `json:"data"`
}

// DecodeOpenRouterManifest validates and normalizes the public models
// response into the compact snapshot used at runtime.
func DecodeOpenRouterManifest(r io.Reader, fetchedAt time.Time) (Snapshot, error) {
	var manifest openRouterManifest
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&manifest); err != nil {
		return Snapshot{}, fmt.Errorf("decode OpenRouter models manifest: %w", err)
	}
	if len(manifest.Data) == 0 {
		return Snapshot{}, fmt.Errorf("decode OpenRouter models manifest: data is empty")
	}

	models := make(map[string]Rate, len(manifest.Data))
	for _, model := range manifest.Data {
		id := normalizeModelID(model.ID)
		if id == "" {
			return Snapshot{}, fmt.Errorf("decode OpenRouter models manifest: model id is empty")
		}
		if _, exists := models[id]; exists {
			return Snapshot{}, fmt.Errorf("decode OpenRouter models manifest: duplicate model %q", id)
		}
		// Router aliases such as openrouter/auto-beta use -1 to mean that
		// the selected downstream model determines the price. They have no
		// fixed rate that can safely enter the versioned snapshot.
		if hasVariablePrice(model.Pricing) {
			continue
		}
		if err := validateRate(model.Pricing); err != nil {
			return Snapshot{}, fmt.Errorf("decode OpenRouter models manifest: model %s: %w", id, err)
		}
		if hasSupportedRate(model.Pricing) {
			models[id] = model.Pricing
		}
	}

	return Snapshot{
		Source:      OPENROUTER_MODELS_URL,
		PricingAsOf: fetchedAt.UTC().Format(time.RFC3339),
		Models:      models,
	}, nil
}

// Estimate converts usage to cost using exact decimal arithmetic. Missing
// billing dimensions fail closed as unpriced.
func (s Snapshot) Estimate(providerName, modelName string, usage core.TokenUsage) core.Cost {
	if strings.EqualFold(strings.TrimSpace(providerName), "ollama") {
		return core.FreeCost()
	}
	if !validUsage(usage) {
		return core.UnpricedCost()
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 &&
		usage.InputCacheReadTokens == 0 && usage.WebSearchCount == 0 {
		return core.UnpricedCost()
	}
	rate, ok := s.Models[openRouterModelID(providerName, modelName)]
	if !ok {
		return core.UnpricedCost()
	}
	rate = rateForInput(rate, usage.InputTokens)
	nonCachedInput := usage.InputTokens - usage.InputCacheReadTokens
	components := []struct {
		units int
		rate  string
	}{
		{units: nonCachedInput, rate: rate.Prompt},
		{units: usage.InputCacheReadTokens, rate: rate.InputCacheRead},
		{units: usage.OutputTokens, rate: rate.Completion},
		{units: usage.WebSearchCount, rate: rate.WebSearch},
	}

	total := new(big.Rat)
	allUsedRatesZero := true
	for _, component := range components {
		if component.units == 0 {
			continue
		}
		if component.rate == "" {
			return core.UnpricedCost()
		}
		value, ok := new(big.Rat).SetString(component.rate)
		if !ok || value.Sign() < 0 {
			return core.UnpricedCost()
		}
		if value.Sign() != 0 {
			allUsedRatesZero = false
		}
		value.Mul(value, new(big.Rat).SetInt64(int64(component.units)))
		total.Add(total, value)
	}

	status := core.COST_STATUS_ESTIMATED
	if allUsedRatesZero {
		status = core.COST_STATUS_FREE
	}
	return core.Cost{
		AmountUSD:   formatUSD(total),
		Status:      status,
		PricingAsOf: s.PricingAsOf,
		Source:      s.Source,
	}
}

func validUsage(usage core.TokenUsage) bool {
	return usage.InputTokens >= 0 &&
		usage.OutputTokens >= 0 &&
		usage.InputCacheReadTokens >= 0 &&
		usage.InputCacheReadTokens <= usage.InputTokens &&
		usage.WebSearchCount >= 0
}

func rateForInput(base Rate, inputTokens int) Rate {
	selected := base
	best := -1
	for _, override := range base.Overrides {
		if override.MinPromptTokens > inputTokens || override.MinPromptTokens < best {
			continue
		}
		best = override.MinPromptTokens
		if override.Prompt != "" {
			selected.Prompt = override.Prompt
		}
		if override.Completion != "" {
			selected.Completion = override.Completion
		}
		if override.WebSearch != "" {
			selected.WebSearch = override.WebSearch
		}
		if override.InputCacheRead != "" {
			selected.InputCacheRead = override.InputCacheRead
		}
	}
	return selected
}

func validateRate(rate Rate) error {
	values := []string{rate.Prompt, rate.Completion, rate.WebSearch, rate.InputCacheRead}
	for _, override := range rate.Overrides {
		if override.MinPromptTokens < 0 {
			return fmt.Errorf("min_prompt_tokens must not be negative")
		}
		values = append(values, override.Prompt, override.Completion, override.WebSearch, override.InputCacheRead)
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if !isNonNegativeDecimal(value) {
			return fmt.Errorf("pricing value %q must be a non-negative decimal string", value)
		}
	}
	return nil
}

func isNonNegativeDecimal(value string) bool {
	dot := false
	digit := false
	for _, r := range value {
		switch {
		case unicode.IsDigit(r):
			digit = true
		case r == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	if !digit {
		return false
	}
	parsed, ok := new(big.Rat).SetString(value)
	return ok && parsed.Sign() >= 0
}

func hasSupportedRate(rate Rate) bool {
	return rate.Prompt != "" || rate.Completion != "" ||
		rate.WebSearch != "" || rate.InputCacheRead != ""
}

func hasVariablePrice(rate Rate) bool {
	values := []string{rate.Prompt, rate.Completion, rate.WebSearch, rate.InputCacheRead}
	for _, override := range rate.Overrides {
		values = append(values, override.Prompt, override.Completion, override.WebSearch, override.InputCacheRead)
	}
	for _, value := range values {
		parsed, ok := new(big.Rat).SetString(value)
		if ok && parsed.Sign() < 0 {
			return true
		}
	}
	return false
}

func openRouterModelID(providerName, modelName string) string {
	model := normalizeModelID(modelName)
	if strings.Contains(model, "/") {
		return model
	}
	author := normalizeModelID(providerName)
	switch author {
	case "grok":
		author = "x-ai"
	case "codex":
		author = "openai"
	}
	if author == "" || model == "" {
		return ""
	}
	return author + "/" + model
}

func normalizeModelID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func formatUSD(value *big.Rat) string {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(core.USD_DECIMAL_PLACES), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(scale))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(scaled.Num(), scaled.Denom(), remainder)
	doubledRemainder := new(big.Int).Lsh(new(big.Int).Abs(remainder), 1)
	if doubledRemainder.Cmp(scaled.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(quotient, scale, fraction)
	return fmt.Sprintf("%s.%0*d", whole.String(), core.USD_DECIMAL_PLACES, fraction)
}

//go:embed snapshot.json
var snapshotFS embed.FS

// Default loads the versioned snapshot embedded in the SDK build.
func Default() Snapshot {
	raw, err := snapshotFS.ReadFile("snapshot.json")
	if err != nil {
		return Snapshot{Models: map[string]Rate{}}
	}
	var snapshot Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot.Models == nil {
		return Snapshot{Models: map[string]Rate{}}
	}
	return snapshot
}
