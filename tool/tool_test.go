package tool

import (
	"testing"

	"github.com/bizshuk/agentsdk/action"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterDefaults_AllToolsRegistered(t *testing.T) {
	reg := action.NewRegistry()
	tools, err := RegisterDefaults(reg, Options{
		Policy:     action.DefaultPolicy(),
		WorkingDir: t.TempDir(),
	})

	require.NoError(t, err)
	assert.Len(t, tools, 6)

	names := []string{"read", "write", "edit", "bash", "glob", "grep"}
	for _, name := range names {
		tool, ok := reg.Get(name)
		assert.True(t, ok, "tool %q should be registered", name)
		assert.NotNil(t, tool)
		assert.Equal(t, name, tool.Name())
	}
}

func TestRegisterDefaults_NilPolicy_WriteEditBashError(t *testing.T) {
	reg := action.NewRegistry()
	tools, err := RegisterDefaults(reg, Options{
		Policy:     nil,
		WorkingDir: t.TempDir(),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "write:")
	assert.Contains(t, err.Error(), "edit:")
	assert.Contains(t, err.Error(), "bash:")
	// Read, Glob, Grep are still registered (3 of 6 succeed).
	assert.GreaterOrEqual(t, len(tools), 3)
}

func TestRegisterDefaults_RiskLevels(t *testing.T) {
	reg := action.NewRegistry()
	_, err := RegisterDefaults(reg, Options{
		Policy:     action.DefaultPolicy(),
		WorkingDir: t.TempDir(),
	})
	require.NoError(t, err)

	// Read-only tools: LOW.
	for _, name := range []string{"read", "glob", "grep"} {
		tl, ok := reg.Get(name)
		require.True(t, ok)
		assert.Equal(t, "low", string(tl.Risk()), "tool %q risk should be low", name)
	}

	// Mutating tools: HIGH.
	for _, name := range []string{"write", "edit", "bash"} {
		tl, ok := reg.Get(name)
		require.True(t, ok)
		assert.Equal(t, "high", string(tl.Risk()), "tool %q risk should be high", name)
	}
}

func TestRegisterDefaults_ToolNotFound(t *testing.T) {
	reg := action.NewRegistry()
	_, err := RegisterDefaults(reg, Options{
		Policy:     action.DefaultPolicy(),
		WorkingDir: t.TempDir(),
	})
	require.NoError(t, err)

	_, ok := reg.Get("nonexistent_tool")
	assert.False(t, ok)
}

func TestRegisterDefaults_SchemasAreNotEmpty(t *testing.T) {
	reg := action.NewRegistry()
	_, err := RegisterDefaults(reg, Options{
		Policy:     action.DefaultPolicy(),
		WorkingDir: t.TempDir(),
	})
	require.NoError(t, err)

	schemas := reg.List()
	assert.Len(t, schemas, 6)

	for _, s := range schemas {
		assert.NotEmpty(t, s.Name, "schema name must not be empty")
		assert.NotEmpty(t, s.Description, "schema description must not be empty for %s", s.Name)
	}
}

func TestMustPolicy_PanicsOnNil(t *testing.T) {
	assert.Panics(t, func() {
		MustPolicy(nil)
	})
}

func TestMustPolicy_ReturnsOnNonNil(t *testing.T) {
	p := action.DefaultPolicy()
	assert.Equal(t, p, MustPolicy(p))
}
