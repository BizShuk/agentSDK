package benchmark

import (
	"slices"
	"strings"

	"github.com/bizshuk/agentsdk/provider"
)

var benchmarkCapabilities = []provider.Capability{
	provider.CAPABILITY_CHAT,
	provider.CAPABILITY_IMAGE,
	provider.CAPABILITY_SPEECH,
	provider.CAPABILITY_TRANSCRIBE,
	provider.CAPABILITY_VIDEO,
	provider.CAPABILITY_MUSIC,
}

type applicabilityKey struct {
	provider   string
	model      string
	capability provider.Capability
}

// benchmarkExclusions records models whose API surface needs an input the
// current predefined case set cannot provide.
var benchmarkExclusions = map[applicabilityKey]struct{}{
	{provider: "minimax", model: "S2V-01", capability: provider.CAPABILITY_VIDEO}:      {},
	{provider: "minimax", model: "music-cover", capability: provider.CAPABILITY_MUSIC}: {},
}

// BenchmarkCapabilities returns the provider capabilities exercised by the
// predefined benchmark case sets, in execution order.
func BenchmarkCapabilities() []provider.Capability {
	return slices.Clone(benchmarkCapabilities)
}

// CatalogSpecs returns the provider's bundled catalog, or nil when the
// provider is unknown or ships none.
func CatalogSpecs(name string) []provider.ModelSpec {
	specs, ok := provider.Catalog(name)
	if !ok {
		return nil
	}
	return specs
}

// CasesForCapability returns a fresh predefined case set for capability.
func CasesForCapability(capability provider.Capability) []Case {
	switch capability {
	case provider.CAPABILITY_CHAT:
		return ChatCases()
	case provider.CAPABILITY_IMAGE:
		return ImageCases()
	case provider.CAPABILITY_SPEECH:
		return SpeechCases()
	case provider.CAPABILITY_TRANSCRIBE:
		return TranscribeCases()
	case provider.CAPABILITY_VIDEO:
		return VideoCases()
	case provider.CAPABILITY_MUSIC:
		return MusicCases()
	default:
		return nil
	}
}

// RunnableCapabilities derives the benchmark subset supported by both the
// provider entry and one catalog model. Exact exclusions cover model APIs the
// predefined cases cannot drive.
func RunnableCapabilities(entry provider.Entry, spec provider.ModelSpec) []provider.Capability {
	var out []provider.Capability
	for _, capability := range benchmarkCapabilities {
		if !entry.Supports(capability) || !spec.Supports(capability) {
			continue
		}
		if _, excluded := benchmarkExclusions[applicabilityKey{
			provider:   strings.ToLower(strings.TrimSpace(entry.Name)),
			model:      spec.ID,
			capability: capability,
		}]; excluded {
			continue
		}
		out = append(out, capability)
	}
	return out
}

// CasesForModel builds every runnable case for one catalog model, filtering
// cases that require an input modality the model does not declare.
func CasesForModel(entry provider.Entry, spec provider.ModelSpec) []Case {
	var out []Case
	for _, capability := range RunnableCapabilities(entry, spec) {
		for _, testCase := range CasesForCapability(capability) {
			if testCase.RequiredInput != "" &&
				!slices.Contains(spec.InputModalities, testCase.RequiredInput) {
				continue
			}
			if capability != provider.CAPABILITY_CHAT && testCase.Model == "" {
				testCase.Model = spec.ID
			}
			out = append(out, testCase)
		}
	}
	return out
}

// CatalogCases resolves one registered provider-model pair into its runnable
// predefined cases. Unknown or non-runnable pairs return nil.
func CatalogCases(providerName, modelID string) []Case {
	entry, ok := provider.Lookup(providerName)
	if !ok {
		return nil
	}
	for _, spec := range CatalogSpecs(entry.Name) {
		if spec.ID == modelID {
			return CasesForModel(entry, spec)
		}
	}
	return nil
}
