package core_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
)

func TestAutonomyString(t *testing.T) {
	assert.Equal(t, "L0", core.AUTONOMY_L0.String())
	assert.Equal(t, "L2", core.AUTONOMY_L2.String())
	assert.Equal(t, "L4", core.AUTONOMY_L4.String())
	assert.Equal(t, "L?", core.AutonomyLevel(99).String())
}
