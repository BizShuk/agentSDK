package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	sdkcore "github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite_CreateAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	w, err := NewWrite(WriteOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)

	// First write: create.
	res, err := w.Call(context.Background(), mustMarshal(WriteArgs{Path: path, Content: "hello world"}))
	require.NoError(t, err)
	assert.True(t, res.OK)
	out := unmarshalOutput[WriteOutput](t, res)
	assert.True(t, out.Created)
	assert.Equal(t, int64(11), out.Wrote)

	// Verify file content.
	data, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "hello world", string(data))

	// Second write: overwrite.
	res2, err := w.Call(context.Background(), mustMarshal(WriteArgs{Path: path, Content: "overwritten"}))
	require.NoError(t, err)
	assert.True(t, res2.OK)
	out2 := unmarshalOutput[WriteOutput](t, res2)
	assert.False(t, out2.Created)
}

func TestWrite_SandboxDenied(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWrite(WriteOptions{}, action.DefaultPolicy(), dir)
	require.NoError(t, err)

	// /etc is not in the default allowed prefixes.
	path := "/etc/hosts"
	res, err := w.Call(context.Background(), mustMarshal(WriteArgs{Path: path, Content: "x"}))
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "sandbox denied")
}

func TestWrite_NilPolicy_Error(t *testing.T) {
	_, err := NewWrite(WriteOptions{}, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox Policy")
}

func TestWrite_RelativePath(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWrite(WriteOptions{DefaultMode: 0o644}, testPolicy(dir), dir)
	require.NoError(t, err)

	res, err := w.Call(context.Background(), mustMarshal(WriteArgs{Path: "data.txt", Content: "relative"}))
	require.NoError(t, err)
	assert.True(t, res.OK)

	data, rerr := os.ReadFile(filepath.Join(dir, "data.txt"))
	require.NoError(t, rerr)
	assert.Equal(t, "relative", string(data))
}

func TestWrite_RiskLevel(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWrite(WriteOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)
	assert.Equal(t, sdkcore.RISK_LEVEL_HIGH, w.Risk())
}
