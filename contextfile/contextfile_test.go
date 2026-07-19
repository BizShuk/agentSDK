package contextfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o640))
}

func TestLoadHierarchy(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o750))
	sub := filepath.Join(root, "pkg", "web")
	write(t, filepath.Join(root, "AGENTS.md"), "root rules")
	write(t, filepath.Join(sub, "CLAUDE.md"), "web rules")
	user := t.TempDir()
	write(t, filepath.Join(user, "AGENTS.md"), "user rules")

	text, files, err := Loader{UserDir: user}.Load(sub)
	require.NoError(t, err)
	require.Len(t, files, 3)
	assert.Equal(t, filepath.Join(user, "AGENTS.md"), files[0].Path, "user layer first")
	assert.Equal(t, filepath.Join(root, "AGENTS.md"), files[1].Path, "repo root second")
	assert.Equal(t, filepath.Join(sub, "CLAUDE.md"), files[2].Path, "cwd file last; CLAUDE.md fallback name")
	assert.Contains(t, text, "user rules")
	assert.Contains(t, text, "root rules")
	assert.Contains(t, text, "web rules")
}

func TestLoadNoFilesIsEmpty(t *testing.T) {
	text, files, err := Loader{}.Load(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Empty(t, files)
}

func TestImportExpansionAndCycle(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o750))
	write(t, filepath.Join(root, "AGENTS.md"), "intro\n@docs/extra.md\noutro")
	write(t, filepath.Join(root, "docs", "extra.md"), "extra content\n@../AGENTS.md")

	text, _, err := Loader{}.Load(root)
	require.NoError(t, err)
	assert.Contains(t, text, "extra content")
	assert.Contains(t, text, "import cycle skipped")
	assert.Contains(t, text, "outro")
}

func TestUnresolvableImportStaysVerbatim(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o750))
	write(t, filepath.Join(root, "AGENTS.md"), "see @missing/file.md here\n@nope.md")

	text, _, err := Loader{}.Load(root)
	require.NoError(t, err)
	assert.Contains(t, text, "see @missing/file.md here", "inline mention untouched")
	assert.Contains(t, text, "@nope.md", "missing import untouched")
}

func TestBudgetCap(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o750))
	write(t, filepath.Join(root, "AGENTS.md"), string(make([]byte, 300)))

	text, _, err := Loader{MaxBytes: 100}.Load(root)
	require.NoError(t, err)
	assert.Contains(t, text, "context budget 100 bytes exceeded")
}
