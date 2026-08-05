package antigravity

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizshuk/agentsdk/core"
)

func TestToPartsEncodesMedia(t *testing.T) {
	image := []byte{0x89, 0x50, 0x4e, 0x47}
	audio := []byte{0x49, 0x44, 0x33}

	parts, err := toParts([]core.Part{
		{Kind: core.PART_KIND_PLAIN_TEXT, Text: "what is in this picture?"},
		{Kind: core.PART_KIND_IMAGE, Image: image, ImageMIME: "image/png"},
		{Kind: core.PART_KIND_IMAGE},
		{Kind: core.PART_KIND_AUDIO, Audio: audio},
	}, false)
	require.NoError(t, err)
	require.Len(t, parts, 3, "an empty image part is skipped")

	require.NotNil(t, parts[1].InlineData)
	assert.Equal(t, "image/png", parts[1].InlineData.MIMEType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(image), parts[1].InlineData.Data)

	require.NotNil(t, parts[2].InlineData)
	assert.Equal(t, "audio/mpeg", parts[2].InlineData.MIMEType, "an unnamed audio type falls back")
	assert.Equal(t, base64.StdEncoding.EncodeToString(audio), parts[2].InlineData.Data)
}

// Gemini 3+ rejects a tool call with no thought signature. When the
// transcript carries a signed thought, the call is signed with it; when
// it does not, the documented skip sentinel keeps the turn alive.
func TestToPartsSignsGeminiToolCalls(t *testing.T) {
	signed, err := toParts([]core.Part{
		{Kind: core.PART_KIND_REASONING, Text: "plan", Reasoning: &core.ReasoningState{Signature: "sig-1"}},
		{Kind: core.PART_KIND_TOOL_USE, ToolUse: &core.ToolCall{ID: "call_1", Name: "read"}},
	}, true)
	require.NoError(t, err)
	require.Len(t, signed, 2)
	assert.Equal(t, "sig-1", signed[1].ThoughtSignature)

	unsigned, err := toParts([]core.Part{
		{Kind: core.PART_KIND_TOOL_USE, ToolUse: &core.ToolCall{ID: "call_1", Name: "read"}},
	}, true)
	require.NoError(t, err)
	require.Len(t, unsigned, 1)
	assert.Equal(t, GEMINI_SKIP_SIGNATURE, unsigned[0].ThoughtSignature)

	claude, err := toParts([]core.Part{
		{Kind: core.PART_KIND_TOOL_USE, ToolUse: &core.ToolCall{ID: "call_1", Name: "read"}},
	}, false)
	require.NoError(t, err)
	require.Len(t, claude, 1)
	assert.Empty(t, claude[0].ThoughtSignature, "Claude matches on id, not signature")
}

// An unsigned thought cannot be replayed: the gateway validates the
// signature and rejects the whole request without one.
func TestToPartsDropsUnsignedReasoning(t *testing.T) {
	parts, err := toParts([]core.Part{
		{Kind: core.PART_KIND_REASONING, Text: "scratch"},
		{Kind: core.PART_KIND_PLAIN_TEXT, Text: "answer"},
	}, true)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "answer", parts[0].Text)
}

// A tool result names the tool, not the call id — Gemini matches a
// response to its declaration by name.
func TestToPartsToolResult(t *testing.T) {
	ok, err := toParts([]core.Part{
		{Kind: core.PART_KIND_TOOL_RESULT, ToolResult: &core.ToolResult{
			CallID: "call_1", Name: "read", OK: true, Output: "file body",
		}},
	}, false)
	require.NoError(t, err)
	require.Len(t, ok, 1)
	require.NotNil(t, ok[0].FunctionResponse)
	assert.Equal(t, "read", ok[0].FunctionResponse.Name)
	assert.Equal(t, "call_1", ok[0].FunctionResponse.ID)
	assert.Equal(t, map[string]any{"result": "file body"}, ok[0].FunctionResponse.Response)

	failed, err := toParts([]core.Part{
		{Kind: core.PART_KIND_TOOL_RESULT, ToolResult: &core.ToolResult{
			CallID: "call_2", Name: "read", Error: "no such file",
		}},
	}, false)
	require.NoError(t, err)
	require.Len(t, failed, 1)
	assert.Equal(t, map[string]any{"error": "no such file"}, failed[0].FunctionResponse.Response,
		"the model has to see the failure to decide what to do next")
}

// A message left with no parts after filtering still needs one: the
// gateway rejects an empty content entry.
func TestToContentsNeverEmitsEmptyParts(t *testing.T) {
	contents, err := toContents([]core.Message{{
		Role:  core.ROLE_ASSISTANT,
		Parts: []core.Part{{Kind: core.PART_KIND_REASONING, Text: "unsigned"}},
	}}, "gemini-3.1-pro-high")
	require.NoError(t, err)
	require.Len(t, contents, 1)
	assert.Equal(t, "model", contents[0].Role)
	require.Len(t, contents[0].Parts, 1)
	assert.Equal(t, ".", contents[0].Parts[0].Text)
}
