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

func TestGrep_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	require.NoError(t, os.WriteFile(path, []byte("INFO: start\nERROR: fail\nINFO: done\n"), 0o644))

	g := NewGrep(GrepOptions{}, nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{
		Pattern: `ERROR`,
		Path:    path,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, out.Count)
	assert.Equal(t, "ERROR: fail", out.Matches[0].Text)
}

func TestGrep_Directory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nimport \"fmt\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nvar x = 1\n"), 0o644))

	g := NewGrep(GrepOptions{}, nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{
		Pattern: `package main`,
		Path:    dir,
	})
	require.NoError(t, err)

	assert.Equal(t, 2, out.Count)
}

func TestGrep_FileGlob(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("TODO: fix this"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("TODO: document"), 0o644))

	g := NewGrep(GrepOptions{}, nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{
		Pattern: `TODO`,
		Path:    dir,
		Glob:    "*.go",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, out.Count)
}

func TestGrep_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("HELLO world\n"), 0o644))

	g := NewGrep(GrepOptions{}, nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{
		Pattern:         `hello`,
		Path:            dir,
		CaseInsensitive: true,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, out.Count)
}

func TestGrep_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	g := NewGrep(GrepOptions{}, nil, dir)
	_, err := g.Handle(context.Background(), GrepArgs{
		Pattern: `[invalid`,
		Path:    dir,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex")
}

func TestGrep_NoMatches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("nothing here\n"), 0o644))

	g := NewGrep(GrepOptions{}, nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{
		Pattern: `zzz`,
		Path:    dir,
	})
	require.NoError(t, err)

	assert.Equal(t, 0, out.Count)
}

func TestGrep_RiskLevel(t *testing.T) {
	dir := t.TempDir()
	g := NewGrep(GrepOptions{}, action.DefaultPolicy(), dir)
	assert.Equal(t, "low", string(g.ToolRisk()))
}
