package wizard_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bizshuk/agentsdk/agent"
	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/cmd/wizard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runWizard executes the command with the given args and stdin, returning stdout.
func runWizard(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	wizard.ResetFlags()
	c := wizard.WizardCmd
	var out, errOut bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errOut)
	c.SetIn(strings.NewReader(stdin))
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), errOut.String(), err
}

func TestWizardEveryTierProducesAValidConfig(t *testing.T) {
	for _, tier := range spec.Tiers() {
		t.Run(tier, func(t *testing.T) {
			out, _, err := runWizard(t, "", "-y", "--tier", tier, "-o", "-")
			require.NoError(t, err)
			require.NotEmpty(t, out)

			path := filepath.Join(t.TempDir(), "agent.yaml")
			require.NoError(t, os.WriteFile(path, []byte(out), 0o600))

			cfg, err := agent.LoadFile(path)
			require.NoError(t, err, "the wizard must not emit a config LoadFile rejects")
			assert.Equal(t, tier, cfg.Tier)
		})
	}
}

func TestWizardOneshotStaysNameless(t *testing.T) {
	out, _, err := runWizard(t, "", "-y", "--tier", spec.TIER_ONESHOT, "-o", "-")
	require.NoError(t, err)
	assert.NotContains(t, out, "name:")
	assert.Contains(t, out, "tier: oneshot")
}

func TestWizardHigherTiersGetAName(t *testing.T) {
	out, _, err := runWizard(t, "", "-y", "--tier", spec.TIER_STANDARD, "-o", "-")
	require.NoError(t, err)
	assert.Contains(t, out, "name:")
}

func TestWizardNonInteractiveIsQuiet(t *testing.T) {
	_, errOut, err := runWizard(t, "", "-y", "--tier", spec.TIER_BASIC, "-o", "-")
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(errOut), "-y must not chatter into a script's stderr")
}

func TestWizardEditRoundTripIsLossless(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")

	first, _, err := runWizard(t, "", "-y", "--tier", spec.TIER_FULL, "-o", path)
	require.NoError(t, err)
	assert.Empty(t, first)

	original, err := os.ReadFile(path)
	require.NoError(t, err)

	second, _, err := runWizard(t, "", "-y", "--edit", path, "-o", "-")
	require.NoError(t, err)
	assert.Equal(t, string(original), second,
		"--edit then take every default must reproduce the file exactly")
}

func TestWizardRefusesToClobber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	_, _, err := runWizard(t, "", "-y", "-o", path)
	require.NoError(t, err)

	_, _, err = runWizard(t, "", "-y", "-o", path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	_, _, err = runWizard(t, "", "-y", "-o", path, "--force")
	require.NoError(t, err)
}

func TestWizardWritesJSONWhenAsked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.json")
	_, _, err := runWizard(t, "", "-y", "--tier", spec.TIER_BASIC, "-o", path)
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(string(raw)), "{"),
		"a .json path must produce JSON, not YAML")

	cfg, err := agent.LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, spec.TIER_BASIC, cfg.Tier)
}

func TestWizardUnknownTierFails(t *testing.T) {
	_, _, err := runWizard(t, "", "-y", "--tier", "turbo", "-o", "-")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tier")
}

func TestWizardListsChoicesFromTheSharedMetadata(t *testing.T) {
	cases := []struct {
		key  string
		want []string
	}{
		{"tier", spec.Tiers()},
		{"reasoning.style", spec.Values(spec.StyleChoices())},
		{"model.provider", nil},
		{"safety.mode", spec.Values(spec.VariantChoices("safety.mode"))},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			out, _, err := runWizard(t, "", "--list", tc.key)
			require.NoError(t, err)
			require.NotEmpty(t, out)
			for _, want := range tc.want {
				assert.Contains(t, out, want)
			}
			assert.Contains(t, out, "*", "exactly one entry should be marked as the default")
		})
	}
}

func TestWizardListRejectsAnUnknownField(t *testing.T) {
	_, _, err := runWizard(t, "", "--list", "nonsense")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestWizardInteractiveSelectionByNumberAndByName(t *testing.T) {
	out, _, err := runWizard(t, "2\nanthropic\n", "-o", "-")
	require.NoError(t, err)
	assert.Contains(t, out, "tier: basic")
	assert.Contains(t, out, "provider: anthropic")
}

func TestWizardKeepsCurrentValueOnUnrecognizedInput(t *testing.T) {
	out, errOut, err := runWizard(t, "99\n", "-o", "-")
	require.NoError(t, err)
	assert.Contains(t, errOut, "unrecognized")
	assert.Contains(t, out, "tier: "+spec.DEFAULT_TIER)
}

func TestWizardAlwaysRegistersTheSelectedStyle(t *testing.T) {
	out, _, err := runWizard(t, "2\n\n\n\n6\n1\n", "-o", "-")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "agent.yaml")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o600))
	cfg, err := agent.LoadFile(path)
	require.NoError(t, err)
	assert.Contains(t, cfg.Reasoning.Enable, cfg.Reasoning.Style)
}

func TestWizardPrintGoIsCompilableShaped(t *testing.T) {
	out, _, err := runWizard(t, "", "-y", "--tier", spec.TIER_STANDARD, "-o", "-", "--print-go")
	require.NoError(t, err)
	assert.Contains(t, out, "app.Main(agent.MustNew(agent.Config{")
	assert.Contains(t, out, `Tier: "standard"`)
	assert.Contains(t, out, "agent.Model{Provider:")
	assert.Contains(t, out, "}))")
}
