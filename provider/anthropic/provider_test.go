package anthropic_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider"
	"github.com/bizshuk/agentsdk/provider/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := provider.New("anthropic", provider.Options{})
	assert.Error(t, err)
}

func TestNewFromEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-from-env")

	// This test only exercises construction; fake-server Generate coverage
	// lives in generate_mock_test.go.
	if testing.Short() {
		t.Skip("skipping anthropic adapter construction check under -short")
	}

	p, err := provider.New("anthropic", provider.Options{})
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestNewWithResolvedConfig(t *testing.T) {
	p, err := anthropic.New(provider.ResolvedConfig{Auth: core.Auth{APIKey: "sk-direct"}})
	require.NoError(t, err)
	assert.NotNil(t, p)
}
