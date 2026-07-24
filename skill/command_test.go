package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandCommand(t *testing.T) {
	r, _ := newTestRegistry(t)

	out, err := r.ExpandCommand("fix", "login crashes on empty password")
	require.NoError(t, err)
	assert.Contains(t, out, "Fix the following issue:\n\nlogin crashes on empty password")
	assert.NotContains(t, out, "$ARGUMENTS")

	out, err = r.ExpandCommand("status", "extra note")
	require.NoError(t, err)
	assert.Equal(t, "Report project status.\n\nextra note", out, "args appended when no placeholder")

	_, err = r.ExpandCommand("ghost", "")
	require.Error(t, err)
}
