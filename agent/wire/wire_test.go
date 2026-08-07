package wire

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	decision := core.APPROVAL_DECISION_APPROVE

	in := []Envelope{
		{Type: TYPE_EVENT, Stream: &core.StreamEvent{Kind: core.STREAM_MESSAGE, RunID: "r1", Text: "hi"}},
		{Type: TYPE_APPROVAL_REQUEST, Approval: &core.PendingApproval{ID: "a1", Reason: "high risk"}},
		{Type: TYPE_HUMAN_DECISION, Decision: &decision},
		{Type: TYPE_RESULT, Result: &Result{
			RunID: "r1", Status: core.RUN_STATUS_COMPLETED, Text: "done",
			Usage: core.TokenUsage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12},
			Cost:  core.Cost{AmountUSD: "0.0010000000", Status: core.COST_STATUS_ESTIMATED},
		}},
		{Type: TYPE_ERROR, Error: &ErrorPayload{Message: "boom"}},
	}
	for _, env := range in {
		require.NoError(t, enc.Encode(env))
	}
	assert.Equal(t, len(in), strings.Count(buf.String(), "\n"), "one line per envelope")

	dec := NewDecoder(&buf)
	var out []Envelope
	for {
		env, err := dec.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		out = append(out, env)
	}
	require.Len(t, out, len(in))
	assert.Equal(t, "hi", out[0].Stream.Text)
	assert.Equal(t, "a1", out[1].Approval.ID)
	assert.Equal(t, core.APPROVAL_DECISION_APPROVE, *out[2].Decision)
	assert.Equal(t, core.RUN_STATUS_COMPLETED, out[3].Result.Status)
	assert.Equal(t, 12, out[3].Result.Usage.TotalTokens)
	assert.Equal(t, "0.0010000000", out[3].Result.Cost.AmountUSD)
	assert.Equal(t, "boom", out[4].Error.Message)
}

func TestSinkImplementsEventSink(t *testing.T) {
	var buf bytes.Buffer
	sink := NewSink(&buf)
	sink.Now = func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) }
	var _ core.EventSink = sink

	sink.OnStreamEvent(core.StreamEvent{Kind: core.STREAM_RUN_START, RunID: "r9"})

	env, err := NewDecoder(&buf).Next()
	require.NoError(t, err)
	assert.Equal(t, TYPE_EVENT, env.Type)
	assert.Equal(t, core.STREAM_RUN_START, env.Stream.Kind)
	assert.Equal(t, 2026, env.Ts.Year())
}

func TestRPCRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteResponse(&buf, Response{ID: "1", OK: true, Result: []byte(`{"x":1}`)}))
	require.NoError(t, WriteResponse(&buf, Response{ID: "2", OK: false, Error: "nope"}))

	// Responses and requests share the same framing; decode as requests to
	// prove LF-delimited JSONL survives.
	sc := bufio.NewScanner(&buf)
	var lines int
	for {
		_, err := ReadRequest(sc)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		lines++
	}
	assert.Equal(t, 2, lines)
}

func TestReadRequestSkipsBlankLines(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader("\n\n{\"id\":\"7\",\"method\":\"prompt\"}\n"))
	req, err := ReadRequest(sc)
	require.NoError(t, err)
	assert.Equal(t, "7", req.ID)
	assert.Equal(t, "prompt", req.Method)
}

func TestFormatStream(t *testing.T) {
	tests := []struct {
		name string
		ev   core.StreamEvent
		want string
	}{
		{"message", core.StreamEvent{Kind: core.STREAM_MESSAGE, Text: "hello"}, "hello"},
		{"tool start", core.StreamEvent{Kind: core.STREAM_TOOL_START, ToolCall: &core.ToolCall{Name: "Bash"}}, "→ Bash"},
		{"tool ok", core.StreamEvent{Kind: core.STREAM_TOOL_RESULT, ToolResult: &core.ToolResult{Name: "Bash", OK: true}}, "← Bash ok"},
		{"tool error", core.StreamEvent{Kind: core.STREAM_TOOL_RESULT, ToolResult: &core.ToolResult{Name: "Bash", Error: "denied"}}, "← Bash error: denied"},
		{"run start silent", core.StreamEvent{Kind: core.STREAM_RUN_START}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatStream(tt.ev))
		})
	}
}
