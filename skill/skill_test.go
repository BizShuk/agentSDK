package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o640))
}

func newTestRegistry(t *testing.T) (*Registry, string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Deploy to staging\nallowed-tools: [Bash, Read]\n---\n\n# Steps\n\nrun make deploy")
	writeFile(t, filepath.Join(dir, "review", "SKILL.md"),
		"---\ndescription: Review checklist\n---\nbody here")
	writeFile(t, filepath.Join(dir, "commands", "fix.md"),
		"Fix the following issue:\n\n$ARGUMENTS\n\nBe thorough.")
	writeFile(t, filepath.Join(dir, "commands", "status.md"), "Report project status.")
	r := NewRegistry()
	require.NoError(t, r.DiscoverSkills(dir))
	require.NoError(t, r.DiscoverCommands(filepath.Join(dir, "commands")))
	return r, dir
}

func TestDiscoverAndList(t *testing.T) {
	r, _ := newTestRegistry(t)

	skills := r.Skills()
	require.Len(t, skills, 2)
	assert.Equal(t, "deploy", skills[0].Name)
	assert.Equal(t, []string{"Bash", "Read"}, skills[0].AllowedTools)
	assert.Equal(t, "review", skills[1].Name, "folder name used when frontmatter name missing")

	cmds := r.Commands()
	require.Len(t, cmds, 2)
	assert.Equal(t, "fix", cmds[0].Name)
}

func TestProgressiveDisclosure(t *testing.T) {
	r, _ := newTestRegistry(t)

	prompt := r.SystemPrompt()
	assert.Contains(t, prompt, "deploy: Deploy to staging")
	assert.NotContains(t, prompt, "make deploy", "bodies stay out of the system prompt")

	body, err := r.Body("deploy")
	require.NoError(t, err)
	assert.Contains(t, body, "run make deploy")
	assert.NotContains(t, body, "description:", "frontmatter stripped")

	_, err = r.Body("nope")
	require.Error(t, err)
}

func TestExpandCommand(t *testing.T) {
	r, _ := newTestRegistry(t)

	out, err := r.ExpandCommand("fix", "login crashes on empty password")
	require.NoError(t, err)
	assert.Contains(t, out, "Fix the following issue:\n\nlogin crashes on empty password")
	assert.NotContains(t, out, "$ARGUMENTS")

	out, err = r.ExpandCommand("status", "extra note")
	require.NoError(t, err)
	assert.Equal(t, "Report project status.\n\nextra note", out, "args appended when no placeholder")

	_, err = r.ExpandCommand("ghost", "")
	require.Error(t, err)
}

func TestProjectOverridesUser(t *testing.T) {
	r, _ := newTestRegistry(t)
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "deploy", "SKILL.md"),
		"---\nname: deploy\ndescription: Project-specific deploy\n---\nproject body")
	require.NoError(t, r.DiscoverSkills(project))

	skills := r.Skills()
	require.Len(t, skills, 2)
	assert.Equal(t, "Project-specific deploy", skills[0].Description, "later discovery wins")
}

func TestRenderTemplate(t *testing.T) {
	out := RenderTemplate("Deploy {{app}} to {{env}} ({{app}})", map[string]string{"app": "web", "env": "staging"})
	assert.Equal(t, "Deploy web to staging (web)", out)
}
