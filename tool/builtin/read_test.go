package builtin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_TextFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644))

	r := NewRead(nil, dir)
	out, err := r.execute(context.Background(), ReadArgs{Path: path})
	require.NoError(t, err)

	assert.Equal(t, 3, out.Total)
	assert.Len(t, out.Lines, 3)
	assert.Equal(t, "line1", out.Lines[0])
	assert.False(t, out.Truncated)
}

func TestRead_LineRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "range.txt")
	content := "a\nb\nc\nd\ne\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	r := NewRead(nil, dir)
	out, err := r.execute(context.Background(), ReadArgs{Path: path, StartLine: 2, LineCount: 2})
	require.NoError(t, err)

	assert.Equal(t, 5, out.Total)
	assert.Equal(t, []string{"b", "c"}, out.Lines)
	assert.True(t, out.Truncated)
}

func TestRead_FileNotFound(t *testing.T) {
	r := NewRead(nil, t.TempDir())
	_, err := r.execute(context.Background(), ReadArgs{Path: "nonexistent.txt"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open")
}

func TestRead_SandboxDenied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0o644))

	r := NewRead(testPolicy(dir), dir)

	_, err := r.execute(context.Background(), ReadArgs{Path: "/etc/passwd"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox denied")
}

func TestRead_BinaryFileRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	// NUL bytes trigger binary detection
	binData := []byte{0x7f, 0x45, 0x4c, 0x46, 0x00, 0x00, 0x00, 0x00}
	require.NoError(t, os.WriteFile(path, binData, 0o644))

	r := NewRead(nil, dir)
	_, err := r.execute(context.Background(), ReadArgs{Path: path})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "binary content")
}
