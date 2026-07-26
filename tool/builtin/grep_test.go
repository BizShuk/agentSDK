package builtin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrep_SubstringSearch(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f1.txt"), []byte("line one\nline target two\nline three\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f2.txt"), []byte("no match here\n"), 0o644))

	g := NewGrep(nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{Query: "target", Cwd: dir})
	require.NoError(t, err)

	assert.Equal(t, 1, out.Count)
	assert.Equal(t, "f1.txt", out.Matches[0].Path)
	assert.Equal(t, 2, out.Matches[0].Line)
	assert.Equal(t, "line target two", out.Matches[0].Content)
	assert.False(t, out.Truncated)
}

func TestGrep_RegexSearch(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "code.go"), []byte("func Foo() {}\nfunc Bar() {}\n"), 0o644))

	g := NewGrep(nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{Query: `func (Foo|Bar)`, Cwd: dir})
	require.NoError(t, err)

	assert.Equal(t, 2, out.Count)
}

func TestGrep_Truncated(t *testing.T) {
	dir := t.TempDir()
	content := "match line\nmatch line\nmatch line\nmatch line\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "many.txt"), []byte(content), 0o644))

	g := NewGrep(nil, dir, WithGrepMaxResults(2))
	out, err := g.Handle(context.Background(), GrepArgs{Query: "match", Cwd: dir})
	require.NoError(t, err)

	assert.Equal(t, 2, out.Count)
	assert.True(t, out.Truncated)
}

func TestGrep_BinarySkipped(t *testing.T) {
	dir := t.TempDir()
	// Binary content should be skipped by grep.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.bin"), []byte("binary target \x00 content"), 0o644))

	g := NewGrep(nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{Query: "target", Cwd: dir})
	require.NoError(t, err)

	assert.Equal(t, 0, out.Count)
}

func TestGrep_EmptyQuery_Error(t *testing.T) {
	g := NewGrep(nil, t.TempDir())
	_, err := g.Handle(context.Background(), GrepArgs{Query: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestGrep_NoMatches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f1.txt"), []byte("nothing here"), 0o644))

	g := NewGrep(nil, dir)
	out, err := g.Handle(context.Background(), GrepArgs{Query: "absent", Cwd: dir})
	require.NoError(t, err)

	assert.Equal(t, 0, out.Count)
}

func TestGrep_RiskLevel(t *testing.T) {
	dir := t.TempDir()
	g := NewGrep(testPolicy(dir), dir)
	assert.Equal(t, "low", string(g.Risk()))
}
