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

func newTestMv(t *testing.T) (*Mv, string) {
	t.Helper()
	dir := t.TempDir()
	m, err := NewMv(testPolicy(dir), dir)
	require.NoError(t, err)
	return m, dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestMv_RenameIntoNewParent(t *testing.T) {
	m, dir := newTestMv(t)
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "nested", "deep", "b.txt")
	writeFile(t, src, "hello")

	out, err := m.execute(context.Background(), MvArgs{Source: src, Target: dst})
	require.NoError(t, err)
	assert.Equal(t, dst, filepath.Join(filepath.Dir(dst), filepath.Base(out.Target)))

	assert.NoFileExists(t, src)
	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestMv_IntoExistingDirectory(t *testing.T) {
	m, dir := newTestMv(t)
	src := filepath.Join(dir, "receipt.jpg")
	dest := filepath.Join(dir, "archive")
	writeFile(t, src, "img")
	require.NoError(t, os.Mkdir(dest, 0o755))

	out, err := m.execute(context.Background(), MvArgs{Source: src, Target: dest})
	require.NoError(t, err)
	assert.Equal(t, "receipt.jpg", filepath.Base(out.Target))
	assert.FileExists(t, filepath.Join(dest, "receipt.jpg"))
	assert.NoFileExists(t, src)
}

func TestMv_RelativePathsResolveAgainstWorkingDir(t *testing.T) {
	m, dir := newTestMv(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "x")

	_, err := m.execute(context.Background(), MvArgs{Source: "a.txt", Target: "b.txt"})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "b.txt"))
}

func TestMv_MovesDirectory(t *testing.T) {
	m, dir := newTestMv(t)
	writeFile(t, filepath.Join(dir, "src", "inner.txt"), "x")

	_, err := m.execute(context.Background(), MvArgs{
		Source: filepath.Join(dir, "src"),
		Target: filepath.Join(dir, "dst"),
	})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "dst", "inner.txt"))
	assert.NoDirExists(t, filepath.Join(dir, "src"))
}

func TestMv_RefusesOverwrite(t *testing.T) {
	m, dir := newTestMv(t)
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")
	writeFile(t, src, "new")
	writeFile(t, dst, "old")

	_, err := m.execute(context.Background(), MvArgs{Source: src, Target: dst})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	data, rerr := os.ReadFile(dst)
	require.NoError(t, rerr)
	assert.Equal(t, "old", string(data))
	assert.FileExists(t, src)
}

func TestMv_RefusesOverwriteInsideDirectory(t *testing.T) {
	m, dir := newTestMv(t)
	src := filepath.Join(dir, "a.txt")
	writeFile(t, src, "new")
	writeFile(t, filepath.Join(dir, "archive", "a.txt"), "old")

	_, err := m.execute(context.Background(), MvArgs{Source: src, Target: filepath.Join(dir, "archive")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.FileExists(t, src)
}

func TestMv_MissingSource(t *testing.T) {
	m, dir := newTestMv(t)
	_, err := m.execute(context.Background(), MvArgs{
		Source: filepath.Join(dir, "nope.txt"),
		Target: filepath.Join(dir, "b.txt"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat source")
}

func TestMv_SandboxDenied(t *testing.T) {
	m, dir := newTestMv(t)
	src := filepath.Join(dir, "a.txt")
	writeFile(t, src, "x")

	t.Run("target outside", func(t *testing.T) {
		_, err := m.execute(context.Background(), MvArgs{Source: src, Target: "/etc/a.txt"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sandbox denied")
		assert.FileExists(t, src)
	})
	t.Run("source outside", func(t *testing.T) {
		_, err := m.execute(context.Background(), MvArgs{Source: "/etc/hosts", Target: filepath.Join(dir, "hosts")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sandbox denied")
	})
	t.Run("symlink source pointing outside", func(t *testing.T) {
		link := filepath.Join(dir, "link")
		require.NoError(t, os.Symlink("/etc/hosts", link))
		_, err := m.execute(context.Background(), MvArgs{Source: link, Target: filepath.Join(dir, "moved")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sandbox denied")
	})
}

func TestMv_NilPolicy_Error(t *testing.T) {
	_, err := NewMv(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox Policy")
}

func TestMv_RiskLevel(t *testing.T) {
	m, _ := newTestMv(t)
	assert.Equal(t, sdkcore.RISK_LEVEL_HIGH, m.Spec().Risk)
}
