package observability_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"github.com/bizshuk/agentsdk/middleware/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingEmitsOneSpanPerEffect(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := newTracerProviderWithRecorder(rec)

	mw := observability.Tracing(observability.TracingConfig{TracerProvider: tp})

	d := func(_ context.Context, _ core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
		return core.State{}, &core.Event{Kind: core.EVENT_TOOL_RESULT}, false, nil
	}

	eff := core.Instruction{Kind: core.INSTRUCTION_CALL_TOOL, CallTool: &core.CallToolInstruction{
		Call: core.ToolCall{ID: "c1", Name: "read_log_tail", Args: map[string]any{"n": 5}, Risk: core.RISK_LEVEL_LOW},
	}}
	_, _, _, err := mw(middleware.Next(d))(context.Background(), core.State{}, eff)
	require.NoError(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "tool.read_log_tail", spans[0].Name())
}

func TestTracingAttributesCarried(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := newTracerProviderWithRecorder(rec)
	mw := observability.Tracing(observability.TracingConfig{TracerProvider: tp})

	d := func(_ context.Context, _ core.State, _ core.Instruction) (core.State, *core.Event, bool, error) {
		return core.State{}, &core.Event{Kind: core.EVENT_TOOL_RESULT}, false, nil
	}
	eff := core.Instruction{Kind: core.INSTRUCTION_CALL_TOOL, CallTool: &core.CallToolInstruction{
		Call: core.ToolCall{ID: "c1", Name: "add_todo", Args: map[string]any{"title": "x"}, Risk: core.RISK_LEVEL_HIGH},
	}}
	_, _, _, err := mw(middleware.Next(d))(context.Background(), core.State{}, eff)
	require.NoError(t, err)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	attrs := spans[0].Attributes()
	haveAttr := func(key string) bool {
		for _, a := range attrs {
			if string(a.Key) == key {
				return true
			}
		}
		return false
	}
	assert.True(t, haveAttr("agentsdk.tool.name"))
	assert.True(t, haveAttr("agentsdk.tool.call_id"))
	assert.True(t, haveAttr("agentsdk.tool.risk"))
}

func TestTracingMarksErrorOnDispatchFailure(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := newTracerProviderWithRecorder(rec)
	mw := observability.Tracing(observability.TracingConfig{TracerProvider: tp})

	d := func(_ context.Context, _ core.State, _ core.Instruction) (core.State, *core.Event, bool, error) {
		return core.State{}, nil, false, errors.New("dispatch failed")
	}
	_, _, _, _ = mw(middleware.Next(d))(context.Background(), core.State{},
		core.Instruction{Kind: core.INSTRUCTION_CALL_MODEL, CallModel: &core.CallModelInstruction{RequestID: "r1"}})

	spans := rec.Ended()
	require.Len(t, spans, 1)
	// TracerError description shows the recorded error.
	status := spans[0].Status()
	assert.NotEmpty(t, status.Description)
}