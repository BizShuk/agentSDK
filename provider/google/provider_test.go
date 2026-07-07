package google_test

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/provider/google"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	_, err := google.New(context.Background())
	assert.Error(t, err)
}

func TestNewWithOption(t *testing.T) {
	p, err := google.New(context.Background(), google.WithAPIKey("AIza-test"))
	require.NoError(t, err)
	assert.Equal(t, "google:gemini-2.0-flash", p.Name())
}

func TestCountTokens(t *testing.T) {
	p, err := google.New(context.Background(), google.WithAPIKey("AIza-test"))
	require.NoError(t, err)
	n, err := p.CountTokens(context.Background(), []core.Message{
		{Role: core.ROLE_USER, Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: "hello"}}},
	})
	require.NoError(t, err)
	assert.Greater(t, n, 0)
}