package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sdkcore "github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEdit_SingleReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("debug: false\nport: 8080\n"), 0o644))

	e, err := NewEdit(EditOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)

	res, cerr := e.Call(context.Background(), mustMarshal(EditArgs{
		Path:    path,
		OldText: "debug: false",
		NewText: "debug: true",
	}))
	require.NoError(t, cerr)
	assert.True(t, res.OK, "edit should succeed, got: %s", res.Error)

	out := unmarshalOutput[EditOutput](t, res)
	assert.Equal(t, 1, out.Replacements)

	data, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Contains(t, string(data), "debug: true")
	assert.NotContains(t, string(data), "debug: false")
}

func TestEdit_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "text.txt")
	require.NoError(t, os.WriteFile(path, []byte("foo bar foo baz foo\n"), 0o644))

	e, err := NewEdit(EditOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)

	res, cerr := e.Call(context.Background(), mustMarshal(EditArgs{
		Path:       path,
		OldText:    "foo",
		NewText:    "qux",
		ReplaceAll: true,
	}))
	require.NoError(t, cerr)
	assert.True(t, res.OK)

	out := unmarshalOutput[EditOutput](t, res)
	assert.Equal(t, 3, out.Replacements)

	data, _ := os.ReadFile(path)
	assert.NotContains(t, string(data), "foo")
	assert.Equal(t, 3, countIn(string(data), "qux"))
}

func TestEdit_EmptyOldText_Error(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(EditOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)

	res, cerr := e.Call(context.Background(), mustMarshal(EditArgs{
		Path:    filepath.Join(dir, "f.txt"),
		OldText: "",
		NewText: "x",
	}))
	require.NoError(t, cerr)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "old_text must not be empty")
}

func TestEdit_NotFound_Error(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	e, err := NewEdit(EditOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)

	res, cerr := e.Call(context.Background(), mustMarshal(EditArgs{
		Path:    path,
		OldText: "not found anywhere",
		NewText: "x",
	}))
	require.NoError(t, cerr)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "not found")
}

func TestEdit_MultipleMatches_WithoutReplaceAll_Error(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("dup dup\n"), 0o644))

	e, err := NewEdit(EditOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)

	res, cerr := e.Call(context.Background(), mustMarshal(EditArgs{
		Path:    path,
		OldText: "dup",
		NewText: "xxx",
	}))
	require.NoError(t, cerr)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "matches 2 times")
}

func TestEdit_SandboxDenied(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(EditOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)

	res, cerr := e.Call(context.Background(), mustMarshal(EditArgs{
		Path:    "/etc/hosts",
		OldText: "x",
		NewText: "y",
	}))
	require.NoError(t, cerr)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "sandbox denied")
}

func TestEdit_NilPolicy_Error(t *testing.T) {
	_, err := NewEdit(EditOptions{}, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox Policy")
}

func TestEdit_RiskLevel(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(EditOptions{}, testPolicy(dir), dir)
	require.NoError(t, err)
	assert.Equal(t, sdkcore.RISK_LEVEL_HIGH, e.Risk())
}

func countIn(s, substr string) int {
	n := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			n++
		}
	}
	return n
}
