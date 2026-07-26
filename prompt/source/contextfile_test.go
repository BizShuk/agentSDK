package source_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/prompt"
	"github.com/bizshuk/agentsdk/prompt/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sectionsOfContextFile(t *testing.T, s prompt.Source, req prompt.Req) []prompt.Section {
	t.Helper()
	got, err := s.Sections(context.Background(), req)
	require.NoError(t, err)
	return got
}

func TestContextFileSourceReadsTheHierarchy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# project rules\n\nbe careful"), 0o600))

	got := sectionsOfContextFile(t, source.ContextFileSource(""), prompt.Req{Cwd: dir})
	require.Len(t, got, 1)
	assert.Equal(t, prompt.ORDER_FILES, got[0].Order)
	assert.Contains(t, got[0].Text, "be careful")
}

func TestContextFileSourceMissingFilesIsNotAnError(t *testing.T) {
	// An agent must run in a directory with no AGENTS.md at all.
	got := sectionsOfContextFile(t, source.ContextFileSource(""), prompt.Req{Cwd: t.TempDir()})
	for _, s := range got {
		assert.Empty(t, strings.TrimSpace(s.Text))
	}
}
