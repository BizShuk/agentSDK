package openaiimage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadResponseRejectsOversizedPayload(t *testing.T) {
	_, err := readResponse(strings.NewReader("12345"), 4)
	require.Error(t, err)
	assert.ErrorContains(t, err, "exceeds 4 bytes")
}

func TestReadErrorResponseKeepsOnlyBoundedPrefix(t *testing.T) {
	raw, err := readErrorResponse(strings.NewReader("12345"), 4)
	require.NoError(t, err)
	assert.Equal(t, "1234", string(raw))
}
