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
	out, err := r.Handle(context.Background(), ReadArgs{Path: path})
	require.NoError(t, err)

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

	out, err := r.Handle(context.Background(), ReadArgs{Path: path, Offset: 0, Limit: 3})
	require.NoError(t, err)

	assert.True(t, out.Truncated, "should be truncated with 3 lines out of 10")
	assert.Equal(t, "text", out.Encoding)
}

func TestRead_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	r := NewRead(ReadOptions{}, testPolicy(dir), dir)

	_, err := r.Handle(context.Background(), ReadArgs{Path: filepath.Join(dir, "nope.txt")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open")
}

func TestRead_SandboxDenied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0o644))

	r := NewRead(ReadOptions{}, action.DefaultPolicy(), dir)

	_, err := r.Handle(context.Background(), ReadArgs{Path: path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox denied")
}

func TestRead_NilPolicy_Allowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "open.txt")
	require.NoError(t, os.WriteFile(path, []byte("open"), 0o644))

	r := NewRead(ReadOptions{}, nil, dir)
	out, err := r.Handle(context.Background(), ReadArgs{Path: path})
	require.NoError(t, err)
	assert.Equal(t, "text", out.Encoding)
}

func TestRead_RelativePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rel.txt"), []byte("relative"), 0o644))

	r := NewRead(ReadOptions{}, nil, dir)
	out, err := r.Handle(context.Background(), ReadArgs{Path: "rel.txt"})
	require.NoError(t, err)
	assert.Equal(t, "relative", out.Content)
}
