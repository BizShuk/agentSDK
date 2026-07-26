package builtin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlob_MatchFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0o644))

	g := NewGlob(nil, dir)
	out, err := g.execute(context.Background(), GlobArgs{Pattern: "*.go", Cwd: dir})
	require.NoError(t, err)

	assert.Equal(t, 2, out.Count)
	assert.Contains(t, out.Matches, "a.go")
	assert.Contains(t, out.Matches, "b.go")
	assert.False(t, out.Truncated)
}

func TestGlob_Truncated(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.Base(t.Name())+string(rune('a'+i))+".tmp"), []byte("x"), 0o644))
	}

	g := NewGlob(nil, dir, WithGlobMaxMatches(2))
	out, err := g.execute(context.Background(), GlobArgs{Pattern: "*.tmp", Cwd: dir})
	require.NoError(t, err)

	assert.Equal(t, 2, out.Count)
	assert.True(t, out.Truncated)
}

func TestGlob_EmptyPattern_Error(t *testing.T) {
	g := NewGlob(nil, t.TempDir())
	_, err := g.execute(context.Background(), GlobArgs{Pattern: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestGlob_NoMatches(t *testing.T) {
	dir := t.TempDir()
	g := NewGlob(nil, dir)
	out, err := g.execute(context.Background(), GlobArgs{Pattern: "*.nonexistent", Cwd: dir})
	require.NoError(t, err)

	assert.Equal(t, 0, out.Count)
}

func TestGlob_RiskLevel(t *testing.T) {
	dir := t.TempDir()
	g := NewGlob(testPolicy(dir), dir)
	assert.Equal(t, "low", string(g.Spec().Risk))
}
