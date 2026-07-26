package core_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
)

func TestRunStatusTerminal(t *testing.T) {
	assert.False(t, core.RUN_STATUS_RUNNING.Terminal())
	assert.False(t, core.RUN_STATUS_PAUSED_APPROVAL.Terminal())
	assert.True(t, core.RUN_STATUS_COMPLETED.Terminal())
	assert.True(t, core.RUN_STATUS_FAILED.Terminal())
	assert.True(t, core.RUN_STATUS_ABORTED.Terminal())
}
