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

func TestGlob_SimplePattern(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644))

	g := NewGlob(GlobOptions{}, nil, dir)
	res, err := g.Call(context.Background(), mustMarshal(GlobArgs{Pattern: "*.go", Cwd: dir}))
	require.NoError(t, err)
	assert.True(t, res.OK)

	out := unmarshalOutput[GlobOutput](t, res)
	assert.Equal(t, 2, out.Count)
	assert.Contains(t, out.Matches, "a.go")
	assert.Contains(t, out.Matches, "b.go")
	assert.NotContains(t, out.Matches, "c.txt")
}

func TestGlob_Doublestar(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "src", "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "lib.go"), []byte(""), 0o644))

	g := NewGlob(GlobOptions{}, nil, dir)
	res, err := g.Call(context.Background(), mustMarshal(GlobArgs{Pattern: "**/*.go", Cwd: dir}))
	require.NoError(t, err)
	assert.True(t, res.OK)

	out := unmarshalOutput[GlobOutput](t, res)
	assert.Equal(t, 2, out.Count)
}

func TestGlob_EmptyPattern(t *testing.T) {
	dir := t.TempDir()
	g := NewGlob(GlobOptions{}, nil, dir)
	res, err := g.Call(context.Background(), mustMarshal(GlobArgs{Pattern: "", Cwd: dir}))
	require.NoError(t, err)
	assert.True(t, res.OK)
}

func TestGlob_NoMatches(t *testing.T) {
	dir := t.TempDir()
	g := NewGlob(GlobOptions{}, nil, dir)
	res, err := g.Call(context.Background(), mustMarshal(GlobArgs{Pattern: "*.nonexistent", Cwd: dir}))
	require.NoError(t, err)
	assert.True(t, res.OK)

	out := unmarshalOutput[GlobOutput](t, res)
	assert.Equal(t, 0, out.Count)
}

func TestGlob_RiskLevel(t *testing.T) {
	dir := t.TempDir()
	g := NewGlob(GlobOptions{}, action.DefaultPolicy(), dir)
	assert.Equal(t, "low", string(g.Risk()))
}
