package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bizshuk/agentsdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePackageUsesSimpleMainEntrypointDeterministically(t *testing.T) {
	root := t.TempDir()
	entry := provider.Entry{
		Name:     "fake",
		Metadata: provider.Metadata{APIKeyEnv: []string{"FAKE_API_KEY"}},
	}
	spec := provider.ModelSpec{
		ID:           "image-1",
		Capabilities: []provider.Capability{provider.CAPABILITY_IMAGE},
	}
	capabilities := []provider.Capability{provider.CAPABILITY_IMAGE}

	require.NoError(t, writePackage(root, "fake-image", entry, spec, capabilities))
	path := filepath.Join(root, "fake-image", "main.go")
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(first), `benchmark.Main("fake", "image-1")`)
	assert.NotContains(t, string(first), "benchmark.Target{")
	assert.NotContains(t, string(first), "benchmark.CatalogCases(")
	assert.Contains(t, string(first), "(image)")

	require.NoError(t, writePackage(root, "fake-image", entry, spec, capabilities))
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}
