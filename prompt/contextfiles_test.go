package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeContextFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o640))
}

func TestLoadContextFilesHierarchy(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o750))
	sub := filepath.Join(root, "pkg", "web")
	writeContextFile(t, filepath.Join(root, "AGENTS.md"), "root rules")
	writeContextFile(t, filepath.Join(sub, "CLAUDE.md"), "web rules")
	user := t.TempDir()
	writeContextFile(t, filepath.Join(user, "AGENTS.md"), "user rules")

	text, files, err := LoadContextFiles(sub, user)
	require.NoError(t, err)
	require.Len(t, files, 3)
	assert.Equal(t, filepath.Join(user, "AGENTS.md"), files[0].Path, "user layer first")
	assert.Equal(t, filepath.Join(root, "AGENTS.md"), files[1].Path, "repo root second")
	assert.Equal(t, filepath.Join(sub, "CLAUDE.md"), files[2].Path, "cwd file last; CLAUDE.md fallback name")
	assert.Contains(t, text, "user rules")
	assert.Contains(t, text, "root rules")
	assert.Contains(t, text, "web rules")
}

func TestLoadContextFilesNoFilesIsEmpty(t *testing.T) {
	text, files, err := LoadContextFiles(t.TempDir(), "")
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Empty(t, files)
}

func TestLoadContextFilesImportExpansionAndCycle(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o750))
	writeContextFile(t, filepath.Join(root, "AGENTS.md"), "intro\n@docs/extra.md\noutro")
	writeContextFile(t, filepath.Join(root, "docs", "extra.md"), "extra content\n@../AGENTS.md")

	text, _, err := LoadContextFiles(root, "")
	require.NoError(t, err)
	assert.Contains(t, text, "extra content")
	assert.Contains(t, text, "import cycle skipped")
	assert.Contains(t, text, "outro")
}

func TestLoadContextFilesUnresolvableImportStaysVerbatim(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o750))
	writeContextFile(t, filepath.Join(root, "AGENTS.md"), "see @missing/file.md here\n@nope.md")

	text, _, err := LoadContextFiles(root, "")
	require.NoError(t, err)
	assert.Contains(t, text, "see @missing/file.md here", "inline mention untouched")
	assert.Contains(t, text, "@nope.md", "missing import untouched")
}

func TestLoadContextFilesBudgetCap(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o750))
	// Big file that exceeds the hardcoded contextFilesMaxBytes cap.
	writeContextFile(t, filepath.Join(root, "AGENTS.md"), strings.Repeat("x", contextFilesMaxBytes+1))

	text, _, err := LoadContextFiles(root, "")
	require.NoError(t, err)
	assert.Contains(t, text, "context budget")
	assert.Contains(t, text, "bytes exceeded")
}
