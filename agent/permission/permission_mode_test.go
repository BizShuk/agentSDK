package permission_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bizshuk/agentsdk/agent/permission"
	"github.com/bizshuk/agentsdk/agent/spec"
)

// TestModeValuesAreStable pins the runtime permission modes to the serialized
// config vocabulary. Renaming a value is a breaking change for downstream
// operator YAML and JSON files.
func TestModeValuesAreStable(t *testing.T) {
	cases := []struct {
		mode permission.Mode
		want string
	}{
		{permission.MODE_DEFAULT, "default"},
		{permission.MODE_ACCEPT_EDITS, "acceptEdits"},
		{permission.MODE_PLAN, "plan"},
		{permission.MODE_BYPASS, "bypassPermissions"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, string(c.mode),
			"%s spelling must match the historical config vocabulary", c.mode)
	}

	assert.Equal(t, spec.MODE_DEFAULT, string(permission.MODE_DEFAULT))
	assert.Equal(t, spec.MODE_ACCEPT_EDITS, string(permission.MODE_ACCEPT_EDITS))
	assert.Equal(t, spec.MODE_PLAN, string(permission.MODE_PLAN))
	assert.Equal(t, spec.MODE_BYPASS, string(permission.MODE_BYPASS))
}
