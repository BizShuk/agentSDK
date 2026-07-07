package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/cli"
	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodecRoundTripAllMessageTypes(t *testing.T) {
	now := time.Now().UTC()
	envs := []cli.Envelope{
		{Type: cli.MSG_TYPE_OBSERVATION, RunID: "r1", Timestamp: now,
			Observation: &cli.ObservationPayload{ID: "p1", Source: "log", ObservedAt: now, Payload: "ERROR"}},
		{Type: cli.MSG_TYPE_ASSISTANT, RunID: "r1", Timestamp: now,
			Assistant: &cli.AssistantPayload{Text: "hi", StopReason: "end_turn"}},
		{Type: cli.MSG_TYPE_TOOL_CALL, RunID: "r1", Timestamp: now,
			ToolCall: &cli.ToolCallPayload{ID: "c1", Name: "add_todo", Args: map[string]any{"title": "x"}, Risk: "low"}},
		{Type: cli.MSG_TYPE_TOOL_RESULT, RunID: "r1", Timestamp: now,
			ToolResult: &cli.ToolResultPayload{CallID: "c1", Name: "add_todo", OK: true, Output: "todo-1"}},
		{Type: cli.MSG_TYPE_APPROVAL_REQUEST, RunID: "r1", Timestamp: now,
			Approval: &cli.ApprovalPayload{ID: "apr-1", Reason: "high_risk", Risk: "high", Summary: "delete prod", RequestedAt: now}},
		{Type: cli.MSG_TYPE_HUMAN_DECISION, RunID: "r1", Timestamp: now,
			Decision: &cli.DecisionPayload{ApprovalID: "apr-1", Decision: "approve", DecidedBy: "operator", DecidedAt: now}},
		{Type: cli.MSG_TYPE_CHECKPOINT, RunID: "r1", Timestamp: now,
			Checkpoint: &cli.CheckpointPayload{RunID: "r1", Turn: 3, Reason: "auto"}},
		{Type: cli.MSG_TYPE_RESULT, RunID: "r1", Timestamp: now,
			Result: &cli.ResultPayload{Status: "completed", Turn: 4}},
		{Type: cli.MSG_TYPE_ERROR, RunID: "r1", Timestamp: now,
			Error: &cli.ErrorPayload{Message: "kaboom", Kind: "model"}},
	}

	var buf bytes.Buffer
	codec := cli.NewJSONLCodec(nil, &buf)
	for _, e := range envs {
		require.NoError(t, codec.Write(e))
	}
	require.NoError(t, codec.Flush())

	// Verify each line is a valid envelope.
	scanner := bufioSplitLines(buf.Bytes())
	got := 0
	for line := range scanner {
		var env cli.Envelope
		require.NoError(t, json.Unmarshal(line, &env), "line %d", got)
		assert.Equal(t, envs[got].Type, env.Type)
		got++
	}
	assert.Equal(t, len(envs), got)
}

func TestStateRoundTripPreservesMidRunApproval(t *testing.T) {
	state := core.State{
		RunID:        "r-mid",
		Turn:         3,
		Status:       core.RUN_STATUS_PAUSED_APPROVAL,
		ReasoningStyle: core.REASON_REACT,
		Autonomy:     core.AUTONOMY_L2,
		Budget:       core.Budget{MaxTurns: 10, UsedTurns: 3},
		PendingApprovals: []core.PendingApproval{{
			ID: "apr-1", Reason: "high_risk", Risk: core.RISK_LEVEL_HIGH,
			Summary: "delete prod",
			ToolCall: &core.ToolCall{ID: "c1", Name: "delete_prod"},
		}},
	}
	raw, err := json.Marshal(state)
	require.NoError(t, err)

	var out core.State
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Len(t, out.PendingApprovals, 1)
	assert.Equal(t, "apr-1", out.PendingApprovals[0].ID)
	assert.Equal(t, "delete_prod", out.PendingApprovals[0].ToolCall.Name)
	assert.Equal(t, core.RUN_STATUS_PAUSED_APPROVAL, out.Status)
}

func TestImageChunkSurvivesJSONRoundTrip(t *testing.T) {
	// Image bytes — the multimodal abstraction must not corrupt the
	// payload through the codec / state pipeline.
	state := core.State{
		RunID: "r-img",
		Messages: []core.Message{
			{Role: core.ROLE_USER, Parts: []core.Part{
				{Kind: core.PART_KIND_IMAGE, ImageMIME: "image/png", Image: []byte{0x89, 0x50, 0x4e, 0x47}},
			}},
		},
	}
	raw, err := json.Marshal(state)
	require.NoError(t, err)

	var out core.State
	require.NoError(t, json.Unmarshal(raw, &out))
	require.Len(t, out.Messages, 1)
	require.Len(t, out.Messages[0].Parts, 1)
	assert.Equal(t, core.PART_KIND_IMAGE, out.Messages[0].Parts[0].Kind)
	assert.Equal(t, "image/png", out.Messages[0].Parts[0].ImageMIME)
	assert.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, out.Messages[0].Parts[0].Image)
}

func TestCodecWriteErrorSugar(t *testing.T) {
	var buf bytes.Buffer
	codec := cli.NewJSONLCodec(nil, &buf)
	require.NoError(t, cli.WriteError(codec, "r1", "model", "rate limited"))
	require.NoError(t, codec.Flush())
	assert.True(t, strings.Contains(buf.String(), `"kind":"model"`))
}

func TestCodecWriteResultSugar(t *testing.T) {
	var buf bytes.Buffer
	codec := cli.NewJSONLCodec(nil, &buf)
	require.NoError(t, cli.WriteResult(codec, "r1", "completed", 7))
	require.NoError(t, codec.Flush())
	assert.Contains(t, buf.String(), `"status":"completed"`)
	assert.Contains(t, buf.String(), `"turn":7`)
}

// bufioSplitLines emits each newline-terminated byte slice via channel.
func bufioSplitLines(data []byte) <-chan []byte {
	ch := make(chan []byte, 1)
	go func() {
		defer close(ch)
		start := 0
		for i, b := range data {
			if b == '\n' {
				ch <- data[start:i]
				start = i + 1
			}
		}
		if start < len(data) {
			ch <- data[start:]
		}
	}()
	return ch
}