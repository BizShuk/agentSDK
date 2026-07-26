package core_test

import (
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
)

func TestEventKinds(t *testing.T) {
	// Discriminator strings must be stable; downstream runtimes and CLIs depend on them.
	assert.Equal(t, "observation", string(core.EVENT_OBSERVATION))
	assert.Equal(t, "model_reply", string(core.EVENT_MODEL_REPLY))
	assert.Equal(t, "tool_result", string(core.EVENT_TOOL_RESULT))
	assert.Equal(t, "human_decision", string(core.EVENT_HUMAN_DECISION))
	assert.Equal(t, "resume", string(core.EVENT_RESUME))
}
