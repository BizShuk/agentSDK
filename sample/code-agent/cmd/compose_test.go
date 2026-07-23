package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assistantMsg(text string) core.Message {
	return core.Message{
		Role:  core.ROLE_ASSISTANT,
		Parts: []core.Part{{Kind: core.PART_KIND_PLAIN_TEXT, Text: text}},
		Ts:    time.Now().UTC(),
	}
}

func TestLastAssistantText(t *testing.T) {
	s := core.State{Messages: []core.Message{
		userMessage("q1"),
		assistantMsg("a1"),
		userMessage("q2"),
		{Role: core.ROLE_ASSISTANT, Parts: []core.Part{{Kind: core.PART_KIND_TOOL_USE}}}, // tool-only turn
		assistantMsg("a2"),
	}}
	assert.Equal(t, "a2", lastAssistantText(s))
	assert.Equal(t, "", lastAssistantText(core.State{}))
}

func TestFakeProviderScriptThenEcho(t *testing.T) {
	p := newFakeProvider()
	ctx := context.Background()
	req := core.ModelRequest{Messages: []core.Message{userMessage("first ask")}}

	r1, err := p.Generate(ctx, req)
	require.NoError(t, err)
	require.Len(t, r1.ToolCalls, 1)
	assert.Equal(t, "glob", r1.ToolCalls[0].Name)

	r2, err := p.Generate(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "end_turn", r2.StopReason)

	r3, err := p.Generate(ctx, core.ModelRequest{Messages: []core.Message{userMessage("後續問題")}})
	require.NoError(t, err)
	assert.Contains(t, r3.Text, "後續問題", "script 用盡後 echo 最後一則 user 輸入")
}

func TestFormatEvent(t *testing.T) {
	lines := formatEvent(core.StreamEvent{Kind: core.STREAM_MESSAGE, Text: "line1\nline2"})
	require.Len(t, lines, 2)
	assert.Equal(t, "● line1", lines[0])
	assert.Equal(t, "  line2", lines[1])

	lines = formatEvent(core.StreamEvent{Kind: core.STREAM_TOOL_START, ToolCall: &core.ToolCall{
		Name: "bash", Args: map[string]any{"command": "ls -la"},
	}})
	require.Len(t, lines, 1)
	assert.Equal(t, "→ bash ls -la", lines[0])

	assert.Nil(t, formatEvent(core.StreamEvent{Kind: core.STREAM_RUN_START}))
}
