package minimax

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMusicResponseRejectsOversizedPayload(t *testing.T) {
	_, err := readMusicResponse(strings.NewReader("12345"), 4)
	require.Error(t, err)
	assert.ErrorContains(t, err, "exceeds 4 bytes")
}

func TestReadMusicErrorResponseKeepsOnlyBoundedPrefix(t *testing.T) {
	raw, err := readMusicErrorResponse(strings.NewReader("12345"), 4)
	require.NoError(t, err)
	assert.Equal(t, "1234", string(raw))
}
