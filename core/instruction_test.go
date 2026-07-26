package core_test

import (
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstructionTaggedUnionJSON(t *testing.T) {
	instruction := core.Instruction{
		Kind: core.INSTRUCTION_CALL_TOOL,
		CallTool: &core.CallToolInstruction{
			Call: core.ToolCall{ID: "c1", Name: "read_log_tail", Args: map[string]any{"n": 10}},
		},
	}
	raw, err := json.Marshal(instruction)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"call_tool"`)
	assert.Contains(t, string(raw), `"c1"`)
}
