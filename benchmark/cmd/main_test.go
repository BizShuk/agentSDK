package main

import (
	"testing"

	"github.com/bizshuk/agentsdk/benchmark"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectCapabilitiesUsesModelMetadata(t *testing.T) {
	entry, spec := catalogModel(t, "minimax", "image-01")

	got, err := selectCapabilities(entry, &spec, "")

	require.NoError(t, err)
	assert.Equal(t, []provider.Capability{provider.CAPABILITY_IMAGE}, got)
}

func TestSelectCapabilitiesKeepsExplicitUnsupportedCapability(t *testing.T) {
	entry, spec := catalogModel(t, "minimax", "image-01")

	got, err := selectCapabilities(entry, &spec, "chat")

	require.NoError(t, err)
	assert.Equal(t, []provider.Capability{provider.CAPABILITY_CHAT}, got)
}

func TestSelectCapabilitiesRejectsUnknownVocabulary(t *testing.T) {
	entry, spec := catalogModel(t, "minimax", "image-01")

	_, err := selectCapabilities(entry, &spec, "embedding")

	require.ErrorContains(t, err, "unknown capability")
}

func TestCasesForSelectionAppliesModelRequirements(t *testing.T) {
	t.Run("subject video needs unsupported benchmark input", func(t *testing.T) {
		entry, spec := catalogModel(t, "minimax", "S2V-01")
		got := casesForSelection(entry, &spec, []provider.Capability{provider.CAPABILITY_VIDEO})
		assert.Empty(t, got)
	})

	t.Run("text video model receives pinned model", func(t *testing.T) {
		entry, spec := catalogModel(t, "minimax", "MiniMax-H3")
		got := casesForSelection(entry, &spec, []provider.Capability{provider.CAPABILITY_VIDEO})
		require.Len(t, got, 1)
		assert.Equal(t, provider.CAPABILITY_VIDEO, got[0].Capability)
		assert.Equal(t, spec.ID, got[0].Model)
	})
}

func catalogModel(t *testing.T, providerName, modelID string) (provider.Entry, provider.ModelSpec) {
	t.Helper()

	entry, ok := provider.Lookup(providerName)
	require.True(t, ok)
	for _, spec := range entry.Catalog() {
		if spec.ID == modelID {
			return entry, spec
		}
	}
	require.Failf(t, "catalog model missing", "%s/%s", providerName, modelID)
	return provider.Entry{}, provider.ModelSpec{}
}

func TestCasesForSelectionFiltersUnsupportedInputs(t *testing.T) {
	entry, spec := catalogModel(t, "codex", "gpt-5.5")

	got := casesForSelection(entry, &spec, []provider.Capability{provider.CAPABILITY_CHAT})

	require.Len(t, got, 2)
	for _, testCase := range got {
		assert.NotEqual(t, "vision-describe", testCase.Name)
		assert.Equal(t, benchmark.Case{
			Name:       testCase.Name,
			Capability: provider.CAPABILITY_CHAT,
			Prompt:     testCase.Prompt,
		}, testCase)
	}
}
