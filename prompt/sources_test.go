package prompt_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sectionsOf(t *testing.T, s prompt.Source, req prompt.Req) []prompt.Section {
	t.Helper()
	got, err := s.Sections(context.Background(), req)
	require.NoError(t, err)
	return got
}

func TestPersonaSourceUsesTheStableOrder(t *testing.T) {
	got := sectionsOf(t, prompt.PersonaSource("you are terse"), prompt.Req{})
	require.Len(t, got, 1)
	assert.Equal(t, prompt.SLOT_SYSTEM, got[0].Slot)
	assert.Equal(t, prompt.ORDER_PERSONA, got[0].Order,
		"persona changes least often, so it anchors the cacheable prefix")
	assert.Equal(t, "you are terse", got[0].Text)
}

func TestPersonaSourceEmptyContributesNothing(t *testing.T) {
	assert.Empty(t, sectionsOf(t, prompt.PersonaSource(""), prompt.Req{}))
}

func TestContextFileSourceReadsTheHierarchy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# project rules\n\nbe careful"), 0o600))

	got := sectionsOf(t, prompt.ContextFileSource(""), prompt.Req{Cwd: dir})
	require.Len(t, got, 1)
	assert.Equal(t, prompt.ORDER_FILES, got[0].Order)
	assert.Contains(t, got[0].Text, "be careful")
}

func TestContextFileSourceMissingFilesIsNotAnError(t *testing.T) {
	// An agent must run in a directory with no AGENTS.md at all.
	got := sectionsOf(t, prompt.ContextFileSource(""), prompt.Req{Cwd: t.TempDir()})
	for _, s := range got {
		assert.Empty(t, strings.TrimSpace(s.Text))
	}
}

func TestEnvSourceIsLastAmongSystemSections(t *testing.T) {
	got := sectionsOf(t, prompt.EnvSource(), prompt.Req{Cwd: t.TempDir()})
	require.Len(t, got, 1)
	assert.Equal(t, prompt.ORDER_ENV, got[0].Order)
	assert.Greater(t, got[0].Order, prompt.ORDER_SKILLS,
		"env changes every run, so it must not sit inside the cached prefix")
	assert.Contains(t, got[0].Text, "working directory:")
	assert.Contains(t, got[0].Text, "date:")
}

func TestReminderSourceOnlySpeaksNearTheBudget(t *testing.T) {
	cases := []struct {
		name     string
		turn     int
		maxTurns int
		want     bool
	}{
		{"plenty left", 1, 20, false},
		{"getting close", 17, 20, true},
		{"last turn", 19, 20, true},
		{"no budget set", 5, 0, false},
		{"already over", 25, 20, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sectionsOf(t, prompt.ReminderSource(), prompt.Req{
				State: core.State{Turn: tc.turn, Budget: core.Budget{MaxTurns: tc.maxTurns}},
			})
			if !tc.want {
				assert.Empty(t, got, "a reminder on every turn is noise the model learns to ignore")
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, prompt.SLOT_REMINDER, got[0].Slot,
				"reminders ride with the user message, never rewrite the system prompt")
			assert.Contains(t, got[0].Text, "remaining")
		})
	}
}

func TestSourcesAssembleInTheDocumentedOrder(t *testing.T) {
	// The end-to-end shape: persona → files → env, one system message.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("PROJECT_RULES"), 0o600))

	b := prompt.Builder{Sources: []prompt.Source{
		prompt.EnvSource(), // registered first, must still sort last
		prompt.PersonaSource("PERSONA"),
		prompt.ContextFileSource(""),
	}}

	msgs, err := b.Seed(context.Background(), prompt.Req{Cwd: dir, Input: "go"})
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	sys := msgs[0].Parts[0].Text
	iPersona := strings.Index(sys, "PERSONA")
	iFiles := strings.Index(sys, "PROJECT_RULES")
	iEnv := strings.Index(sys, "working directory:")
	require.NotEqual(t, -1, iPersona)
	require.NotEqual(t, -1, iFiles)
	require.NotEqual(t, -1, iEnv)
	assert.Less(t, iPersona, iFiles)
	assert.Less(t, iFiles, iEnv)
}
