package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/reasoning"
	"github.com/bizshuk/agentsdk/tool"
	"github.com/bizshuk/agentsdk/tool/builtin"
)

// TestAutonomyDefaultStringMatchesTyped pins the two-layer default:
// the typed constant (AUTONOMY_DEFAULT) and the string form
// (AUTONOMY_DEFAULT_STRING) must agree with AUTONOMY_L2.String() so
// runtime and config vocabulary stay in lockstep. If a future change
// re-maps the default to a different level, this test fails to call
// the divergence out.
func TestAutonomyDefaultStringMatchesTyped(t *testing.T) {
	assert.Equal(t, core.AUTONOMY_DEFAULT.String(), core.AUTONOMY_DEFAULT_STRING,
		"AUTONOMY_DEFAULT (typed) and AUTONOMY_DEFAULT_STRING (config vocab) must match")
	assert.Equal(t, core.AUTONOMY_L2.String(), core.AUTONOMY_DEFAULT_STRING,
		"AUTONOMY_DEFAULT_STRING must mirror the L2 label so the string form survives a future change")
	assert.Equal(t, spec.DEFAULT_AUTONOMY, core.AUTONOMY_DEFAULT_STRING,
		"spec.DEFAULT_AUTONOMY is the config-file default — it must equal core.AUTONOMY_DEFAULT_STRING")
}

// TestStyleDefaultTracesToCore pins the spec style default against
// the runtime contract. A future change renaming REASON_REACT in core
// would silently desync spec unless this is asserted.
func TestStyleDefaultTracesToCore(t *testing.T) {
	assert.Equal(t, spec.DEFAULT_STYLE, core.REASON_REACT,
		"spec.DEFAULT_STYLE must mirror the runtime contract")
}

// TestBuiltinNamesTracesToSpec pins the built-in tool allowlist across the
// implementation owner and the declarative catalog. Register proves every
// catalog value reaches a constructor without involving agent composition.
func TestBuiltinNamesTracesToSpec(t *testing.T) {
	choices := spec.VariantChoices("tools.builtin")
	for _, n := range builtin.BuiltinNames() {
		found := false
		for _, c := range choices {
			if c.Value == n {
				found = true
				break
			}
		}
		assert.True(t, found,
			"tool %q must appear in spec.VariantChoices('tools.builtin')", n)
	}
	for _, c := range choices {
		found := false
		for _, n := range builtin.BuiltinNames() {
			if n == c.Value {
				found = true
				break
			}
		}
		assert.True(t, found,
			"spec.VariantChoices('tools.builtin') entry %q must have a matching tool.NAME_*", c.Value)

		reg := tool.NewRegistry()
		err := builtin.Register(reg, []string{c.Value}, builtin.Options{
			Policy:     tool.DefaultPolicy(),
			WorkingDir: t.TempDir(),
		})
		require.NoError(t, err, "tool %q must have a built-in constructor", c.Value)
	}
}

func TestReasoningStylesTraceToFactory(t *testing.T) {
	for _, choice := range spec.StyleChoices() {
		rule, err := reasoning.NewRule(choice.Value)
		require.NoError(t, err, "reasoning style %q must have a built-in rule", choice.Value)
		assert.Equal(t, choice.Value, rule.Kind())
	}
}

func TestAutonomyChoicesTraceToCoreParser(t *testing.T) {
	for _, choice := range spec.VariantChoices("limits.autonomy") {
		level, err := core.ParseAutonomyLevel(choice.Value)
		require.NoError(t, err, "autonomy value %q must parse in core", choice.Value)
		assert.Equal(t, choice.Value, level.String())
	}
}
