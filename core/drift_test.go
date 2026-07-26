package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bizshuk/agentsdk/agent/spec"
	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/tool"
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

// TestPermissionModeValuesAreStable pins the spelling of the four
// permission-mode vocabulary strings against typo and rename. The
// strings are part of the config-file format; renaming any of them
// is a breaking change for downstream operator YAML files.
func TestPermissionModeValuesAreStable(t *testing.T) {
	cases := []struct {
		constant string
		want     string
	}{
		{core.PERMISSION_MODE_DEFAULT, "default"},
		{core.PERMISSION_MODE_ACCEPT_EDITS, "acceptEdits"},
		{core.PERMISSION_MODE_PLAN, "plan"},
		{core.PERMISSION_MODE_BYPASS, "bypassPermissions"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, c.constant,
			"%s spelling must match the historical config vocabulary", c.constant)
	}

	// Three-layer reduction check: spec re-exports the same literals.
	assert.Equal(t, spec.MODE_DEFAULT, core.PERMISSION_MODE_DEFAULT)
	assert.Equal(t, spec.MODE_ACCEPT_EDITS, core.PERMISSION_MODE_ACCEPT_EDITS)
	assert.Equal(t, spec.MODE_PLAN, core.PERMISSION_MODE_PLAN)
	assert.Equal(t, spec.MODE_BYPASS, core.PERMISSION_MODE_BYPASS)
}

// TestStyleDefaultTracesToCore pins the spec style default against
// the runtime contract. A future change renaming REASON_REACT in core
// would silently desync spec unless this is asserted.
func TestStyleDefaultTracesToCore(t *testing.T) {
	assert.Equal(t, spec.DEFAULT_STYLE, string(core.REASON_REACT),
		"spec.DEFAULT_STYLE must mirror the runtime contract")
}

// TestBuiltinNamesTracesToSpec pins the built-in tool allowlist across
// three sites: the package that owns the implementation (tool), the
// catalog that the wizard enumerates (spec.VariantChoices), and the
// runtime switch in agent/build.go. A new built-in added to
// tool/BuiltinNames() without an entry in spec.VariantChoices would
// leave the wizard offering no default; a name in spec.VariantChoices
// without a constructor in tool/<name>.go would fail agent/build.go
// at runtime. The cross-check is the only way to catch both.
func TestBuiltinNamesTracesToSpec(t *testing.T) {
	choices := spec.VariantChoices("tools.builtin")
	for _, n := range tool.BuiltinNames() {
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
		for _, n := range tool.BuiltinNames() {
			if n == c.Value {
				found = true
				break
			}
		}
		assert.True(t, found,
			"spec.VariantChoices('tools.builtin') entry %q must have a matching tool.NAME_*", c.Value)
	}
}
