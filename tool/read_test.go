package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_TextFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(path, []byte("line 1\nline 2\nline 3\n"), 0o644))

	r := NewRead(ReadOptions{}, testPolicy(dir), dir)
	res, err := r.Call(context.Background(), mustMarshal(ReadArgs{Path: path}))
	require.NoError(t, err)
	assert.True(t, res.OK, "read should succeed, got error: %s", res.Error)

	out := unmarshalOutput[ReadOutput](t, res)
	assert.Equal(t, "text", out.Encoding)
	assert.Contains(t, out.Content, "line 1")
	assert.False(t, out.Truncated)
}

func TestRead_Pagination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nums.txt")
	content := ""
	for i := range 10 {
		content += "line " + string(rune('0'+i)) + "\n"
	}
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	r := NewRead(ReadOptions{}, testPolicy(dir), dir)

	res, err := r.Call(context.Background(), mustMarshal(ReadArgs{Path: path, Offset: 0, Limit: 3}))
	require.NoError(t, err)
	assert.True(t, res.OK)

	out := unmarshalOutput[ReadOutput](t, res)
	assert.True(t, out.Truncated, "should be truncated with 3 lines out of 10")
	assert.Equal(t, "text", out.Encoding)
}

func TestRead_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	r := NewRead(ReadOptions{}, testPolicy(dir), dir)

	res, err := r.Call(context.Background(), mustMarshal(ReadArgs{Path: filepath.Join(dir, "nope.txt")}))
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "open")
}

func TestRead_SandboxDenied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0o644))

	// Use DefaultPolicy (only allows /tmp) — the temp dir won't be allowed.
	r := NewRead(ReadOptions{}, action.DefaultPolicy(), dir)

	res, err := r.Call(context.Background(), mustMarshal(ReadArgs{Path: path}))
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "sandbox denied")
}

func TestRead_NilPolicy_Allowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "open.txt")
	require.NoError(t, os.WriteFile(path, []byte("open"), 0o644))

	r := NewRead(ReadOptions{}, nil, dir)
	res, err := r.Call(context.Background(), mustMarshal(ReadArgs{Path: path}))
	require.NoError(t, err)
	assert.True(t, res.OK, "read with nil policy should succeed, got: %s", res.Error)
}

func TestRead_RelativePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rel.txt"), []byte("relative"), 0o644))

	r := NewRead(ReadOptions{}, nil, dir)
	res, err := r.Call(context.Background(), mustMarshal(ReadArgs{Path: "rel.txt"}))
	require.NoError(t, err)
	assert.True(t, res.OK, "relative path should be resolved against rootDir")
}
