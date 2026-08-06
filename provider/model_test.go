package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelSpecSupportsDeclaredCapabilities(t *testing.T) {
	t.Parallel()

	spec := provider.ModelSpec{
		ID:           "multi-surface",
		Capabilities: []provider.Capability{provider.CAPABILITY_CHAT, provider.CAPABILITY_IMAGE},
	}

	assert.True(t, spec.Supports(provider.CAPABILITY_CHAT))
	assert.True(t, spec.Supports(provider.CAPABILITY_IMAGE))
	assert.False(t, spec.Supports(provider.CAPABILITY_VIDEO))
}

func TestModelSpecSeparatesInputAndOutputModalities(t *testing.T) {
	t.Parallel()

	spec := provider.ModelSpec{
		ID:               "video-editor",
		InputModalities:  []provider.Modality{provider.MODALITY_TEXT, provider.MODALITY_IMAGE, provider.MODALITY_VIDEO},
		OutputModalities: []provider.Modality{provider.MODALITY_VIDEO},
		Capabilities:     []provider.Capability{provider.CAPABILITY_VIDEO},
		ContextWindow:    32768,
		MaxTokens:        8192,
	}

	raw, err := json.Marshal(spec)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"video-editor",
		"capabilities":["video"],
		"input_modalities":["text","image","video"],
		"output_modalities":["video"],
		"context_window":32768,
		"max_tokens":8192
	}`, string(raw))

	var decoded provider.ModelSpec
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, spec, decoded)
}

func TestModelListerUsesProviderCatalogType(t *testing.T) {
	t.Parallel()

	var lister provider.ModelLister = modelListerStub{}
	specs, err := lister.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []provider.ModelSpec{{ID: "live-model"}}, specs)
}

func TestBundledCatalogCallsReturnIndependentModelMetadata(t *testing.T) {
	for _, entry := range provider.Entries() {
		if entry.Catalog == nil {
			continue
		}
		t.Run(entry.Name, func(t *testing.T) {
			first := entry.Catalog()
			second := entry.Catalog()
			require.NotEmpty(t, first)
			expected := cloneModelSpecs(second)
			require.Equal(t, expected, first)

			first[0].ID = "mutated"
			if len(first[0].Capabilities) > 0 {
				first[0].Capabilities[0] = "mutated"
			}
			if len(first[0].InputModalities) > 0 {
				first[0].InputModalities[0] = "mutated"
			}
			if len(first[0].OutputModalities) > 0 {
				first[0].OutputModalities[0] = "mutated"
			}

			assert.Equal(t, expected, second)
			assert.Equal(t, expected, entry.Catalog())
		})
	}
}

func cloneModelSpecs(specs []provider.ModelSpec) []provider.ModelSpec {
	out := slices.Clone(specs)
	for i := range out {
		out[i].Capabilities = slices.Clone(out[i].Capabilities)
		out[i].InputModalities = slices.Clone(out[i].InputModalities)
		out[i].OutputModalities = slices.Clone(out[i].OutputModalities)
	}
	return out
}

func TestBundledCatalogInvariants(t *testing.T) {
	t.Parallel()

	knownModalities := []provider.Modality{
		provider.MODALITY_TEXT,
		provider.MODALITY_IMAGE,
		provider.MODALITY_AUDIO,
		provider.MODALITY_VIDEO,
	}
	requiredDirection := map[provider.Capability]struct {
		input  provider.Modality
		output provider.Modality
	}{
		provider.CAPABILITY_CHAT:       {input: provider.MODALITY_TEXT, output: provider.MODALITY_TEXT},
		provider.CAPABILITY_IMAGE:      {input: provider.MODALITY_TEXT, output: provider.MODALITY_IMAGE},
		provider.CAPABILITY_VIDEO:      {input: provider.MODALITY_TEXT, output: provider.MODALITY_VIDEO},
		provider.CAPABILITY_MUSIC:      {input: provider.MODALITY_TEXT, output: provider.MODALITY_AUDIO},
		provider.CAPABILITY_TRANSCRIBE: {input: provider.MODALITY_AUDIO, output: provider.MODALITY_TEXT},
		provider.CAPABILITY_SPEECH:     {input: provider.MODALITY_TEXT, output: provider.MODALITY_AUDIO},
		provider.CAPABILITY_LIVE:       {input: provider.MODALITY_TEXT, output: provider.MODALITY_TEXT},
		provider.CAPABILITY_TRANSLATE:  {input: provider.MODALITY_TEXT, output: provider.MODALITY_TEXT},
	}

	for _, entry := range provider.Entries() {
		if entry.Catalog == nil {
			continue
		}
		t.Run(entry.Name, func(t *testing.T) {
			specs := entry.Catalog()
			require.NotEmpty(t, specs)

			seenIDs := make(map[string]struct{}, len(specs))
			covered := make(map[provider.Capability]bool)
			for _, spec := range specs {
				require.NotEmpty(t, spec.ID)
				_, duplicate := seenIDs[spec.ID]
				assert.Falsef(t, duplicate, "duplicate model id %q", spec.ID)
				seenIDs[spec.ID] = struct{}{}

				assertUnique(t, fmt.Sprintf("model %s capabilities", spec.ID), spec.Capabilities)
				assertUnique(t, fmt.Sprintf("model %s input modalities", spec.ID), spec.InputModalities)
				assertUnique(t, fmt.Sprintf("model %s output modalities", spec.ID), spec.OutputModalities)
				for _, modality := range append(slices.Clone(spec.InputModalities), spec.OutputModalities...) {
					assert.Containsf(t, knownModalities, modality, "model %s has unknown modality", spec.ID)
				}
				for _, capability := range spec.Capabilities {
					assert.NotEqual(t, provider.CAPABILITY_CATALOG, capability,
						"catalog is provider-level, not model-level")
					assert.Truef(t, entry.Supports(capability),
						"model %s declares unsupported entry capability %s", spec.ID, capability)
					direction, ok := requiredDirection[capability]
					require.Truef(t, ok, "model %s uses unknown executable capability %s", spec.ID, capability)
					assert.Containsf(t, spec.InputModalities, direction.input,
						"model %s capability %s input", spec.ID, capability)
					assert.Containsf(t, spec.OutputModalities, direction.output,
						"model %s capability %s output", spec.ID, capability)
					covered[capability] = true
				}
			}

			for _, capability := range entry.Capabilities() {
				if capability == provider.CAPABILITY_CATALOG {
					continue
				}
				assert.Truef(t, covered[capability],
					"entry capability %s has no model in the bundled catalog", capability)
			}
		})
	}
}

func assertUnique[T comparable](t *testing.T, label string, values []T) {
	t.Helper()

	seen := make(map[T]struct{}, len(values))
	for _, value := range values {
		_, duplicate := seen[value]
		assert.Falsef(t, duplicate, "%s contains duplicate %v", label, value)
		seen[value] = struct{}{}
	}
}

type modelListerStub struct{}

func (modelListerStub) ListModels(context.Context) ([]provider.ModelSpec, error) {
	return []provider.ModelSpec{{ID: "live-model"}}, nil
}
