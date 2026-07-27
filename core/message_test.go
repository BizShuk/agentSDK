package core_test

import (
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasoningPartJSONRoundTrip(t *testing.T) {
	in := core.Part{
		Kind: core.PART_KIND_REASONING,
		Text: "inspect the repository",
		Reasoning: &core.ReasoningState{
			ID:               "reasoning_1",
			Signature:        "anthropic-signature",
			EncryptedContent: "encrypted-reasoning",
		},
	}

	raw, err := json.Marshal(in)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"kind":"reasoning",
		"text":"inspect the repository",
		"reasoning":{
			"id":"reasoning_1",
			"signature":"anthropic-signature",
			"encrypted_content":"encrypted-reasoning"
		}
	}`, string(raw))

	var out core.Part
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, in, out)
}

func TestModelResultNormalizeContent(t *testing.T) {
	t.Run("parts are authoritative", func(t *testing.T) {
		call := core.ToolCall{ID: "call_1", Name: "read"}
		in := core.ModelResult{
			Parts: []core.Part{
				{Kind: core.PART_KIND_REASONING, Text: "check first"},
				{Kind: core.PART_KIND_PLAIN_TEXT, Text: "done"},
				{Kind: core.PART_KIND_TOOL_USE, ToolUse: &call},
			},
			Text:      "stale",
			ToolCalls: []core.ToolCall{{ID: "stale"}},
		}

		got := in.NormalizeContent()
		assert.Equal(t, "done", got.Text)
		assert.Equal(t, []core.ToolCall{call}, got.ToolCalls)
		assert.Equal(t, in.Parts, got.Parts)
	})

	t.Run("legacy fields synthesize parts", func(t *testing.T) {
		call := core.ToolCall{ID: "call_1", Name: "read"}
		got := (core.ModelResult{Text: "done", ToolCalls: []core.ToolCall{call}}).NormalizeContent()

		require.Len(t, got.Parts, 2)
		assert.Equal(t, core.PART_KIND_PLAIN_TEXT, got.Parts[0].Kind)
		assert.Equal(t, "done", got.Parts[0].Text)
		assert.Equal(t, core.PART_KIND_TOOL_USE, got.Parts[1].Kind)
		require.NotNil(t, got.Parts[1].ToolUse)
		assert.Equal(t, call, *got.Parts[1].ToolUse)
	})
}
