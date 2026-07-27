package anthropic_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/provider/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := anthropic.New()
	assert.Error(t, err)
}

func TestNewFromEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-from-env")

	// This test only exercises construction; fake-server Generate coverage
	// lives in generate_mock_test.go.
	if testing.Short() {
		t.Skip("skipping anthropic adapter construction check under -short")
	}

	p, err := anthropic.New()
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestGenerateWithOption(t *testing.T) {
	p, err := anthropic.New(anthropic.WithAPIKey("sk-direct"))
	require.NoError(t, err)
	assert.NotNil(t, p)
}
