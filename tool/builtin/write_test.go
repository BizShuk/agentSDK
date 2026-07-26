package builtin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sdkcore "github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite_CreateAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWrite(testPolicy(dir), dir)
	require.NoError(t, err)

	path := filepath.Join(dir, "new.txt")

	// First write: created=true.
	out, herr := w.execute(context.Background(), WriteArgs{Path: path, Content: "hello world"})
	require.NoError(t, herr)
	assert.True(t, out.Created)
	assert.Equal(t, int64(11), out.Wrote)

	data, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "hello world", string(data))

	// Second write: overwrite.
	out2, herr2 := w.execute(context.Background(), WriteArgs{Path: path, Content: "overwritten"})
	require.NoError(t, herr2)
	assert.False(t, out2.Created)
}

func TestWrite_SandboxDenied(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWrite(testPolicy(dir), dir)
	require.NoError(t, err)

	// /etc is not in the allowed prefixes of testPolicy(dir).
	path := "/etc/hosts"
	_, herr := w.execute(context.Background(), WriteArgs{Path: path, Content: "x"})
	require.Error(t, herr)
	assert.Contains(t, herr.Error(), "sandbox denied")
}

func TestWrite_NilPolicy_Error(t *testing.T) {
	_, err := NewWrite(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox Policy")
}

func TestWrite_RiskLevel(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWrite(testPolicy(dir), dir)
	require.NoError(t, err)
	assert.Equal(t, sdkcore.RISK_LEVEL_HIGH, w.Spec().Risk)
}
