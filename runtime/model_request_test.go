package runtime

import (
	"context"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/utils/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunInstructionForwardsCanonicalModelRequest(t *testing.T) {
	provider := testutil.NewScriptedProvider()
	provider.EnqueueEndTurn("done")

	request := core.ModelRequest{
		RequestID: "request-1",
		Messages: []core.Message{{
			Role: core.ROLE_USER,
			Parts: []core.Part{{
				Kind: core.PART_KIND_PLAIN_TEXT,
				Text: "hello",
			}},
		}},
		Tools: []core.ToolSpec{{
			Name:       "explicit",
			Parameters: map[string]any{"type": "object"},
			Risk:       core.RISK_LEVEL_LOW,
		}},
		MaxTokens:   321,
		StopReasons: []string{"stop"},
		Auth: core.Auth{
			Bearer:  "token",
			BaseURL: "https://example.invalid",
		},
	}

	engine := NewEngine(nil, provider, nil)
	_, _, paused, err := engine.runInstruction(
		context.Background(),
		core.State{},
		core.Instruction{Kind: core.INSTRUCTION_CALL_MODEL, CallModel: &request},
	)
	require.NoError(t, err)
	assert.False(t, paused)
	assert.Equal(t, request, provider.LastRequest())
}
