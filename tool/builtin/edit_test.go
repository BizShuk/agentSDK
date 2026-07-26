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

func TestEdit_SingleReplacement(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(testPolicy(dir), dir)
	require.NoError(t, err)

	path := filepath.Join(dir, "code.py")
	require.NoError(t, os.WriteFile(path, []byte("def foo():\n    return 1\n"), 0o644))

	out, herr := e.execute(context.Background(), EditArgs{
		Path:    path,
		OldText: "return 1",
		NewText: "return 42",
	})
	require.NoError(t, herr)
	assert.Equal(t, 1, out.Replacements)

	data, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "def foo():\n    return 42\n", string(data))
}

func TestEdit_NotFound_Error(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(testPolicy(dir), dir)
	require.NoError(t, err)

	path := filepath.Join(dir, "code.py")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0o644))

	_, herr := e.execute(context.Background(), EditArgs{
		Path:    path,
		OldText: "missing",
		NewText: "x",
	})
	require.Error(t, herr)
	assert.Contains(t, herr.Error(), "old_text not found")
}

func TestEdit_MultipleMatches_WithoutReplaceAll_Error(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(testPolicy(dir), dir)
	require.NoError(t, err)

	path := filepath.Join(dir, "code.py")
	require.NoError(t, os.WriteFile(path, []byte("foo\nfoo\nfoo\n"), 0o644))

	_, herr := e.execute(context.Background(), EditArgs{
		Path:    path,
		OldText: "foo",
		NewText: "bar",
	})
	require.Error(t, herr)
	assert.Contains(t, herr.Error(), "found 3 times")
}

func TestEdit_MultipleMatches_WithReplaceAll_Success(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(testPolicy(dir), dir)
	require.NoError(t, err)

	path := filepath.Join(dir, "code.py")
	require.NoError(t, os.WriteFile(path, []byte("foo\nfoo\nfoo\n"), 0o644))

	out, herr := e.execute(context.Background(), EditArgs{
		Path:       path,
		OldText:    "foo",
		NewText:    "bar",
		ReplaceAll: true,
	})
	require.NoError(t, herr)
	assert.Equal(t, 3, out.Replacements)

	data, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "bar\nbar\nbar\n", string(data))
}

func TestEdit_EmptyOldText_Error(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(testPolicy(dir), dir)
	require.NoError(t, err)

	_, herr := e.execute(context.Background(), EditArgs{
		Path:    "x.txt",
		OldText: "",
		NewText: "y",
	})
	require.Error(t, herr)
	assert.Contains(t, herr.Error(), "must not be empty")
}

func TestEdit_SandboxDenied(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(testPolicy(dir), dir)
	require.NoError(t, err)

	_, herr := e.execute(context.Background(), EditArgs{
		Path:    "/etc/hosts",
		OldText: "localhost",
		NewText: "x",
	})
	require.Error(t, herr)
	assert.Contains(t, herr.Error(), "sandbox denied")
}

func TestEdit_NilPolicy_Error(t *testing.T) {
	_, err := NewEdit(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox Policy")
}

func TestEdit_RiskLevel(t *testing.T) {
	dir := t.TempDir()
	e, err := NewEdit(testPolicy(dir), dir)
	require.NoError(t, err)
	assert.Equal(t, sdkcore.RISK_LEVEL_HIGH, e.Spec().Risk)
}
