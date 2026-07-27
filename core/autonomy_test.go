package core_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutonomyString(t *testing.T) {
	assert.Equal(t, "L0", core.AUTONOMY_L0.String())
	assert.Equal(t, "L2", core.AUTONOMY_L2.String())
	assert.Equal(t, "L4", core.AUTONOMY_L4.String())
	assert.Equal(t, "L?", core.AutonomyLevel(99).String())
}

func TestParseAutonomyLevel(t *testing.T) {
	tests := map[string]core.AutonomyLevel{
		"L0": core.AUTONOMY_L0,
		"L1": core.AUTONOMY_L1,
		"L2": core.AUTONOMY_L2,
		"L3": core.AUTONOMY_L3,
		"L4": core.AUTONOMY_L4,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := core.ParseAutonomyLevel(input)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.Equal(t, input, got.String())
		})
	}
}

func TestParseAutonomyLevelRejectsUnknownValue(t *testing.T) {
	for _, input := range []string{"", "l2", "L5"} {
		t.Run(input, func(t *testing.T) {
			_, err := core.ParseAutonomyLevel(input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown autonomy level")
		})
	}
}
