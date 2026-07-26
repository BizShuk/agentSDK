// Package observability hosts cross-cutting telemetry middlewares.
//
// tracing.go wraps each effect in an OpenTelemetry span. The runtime
// default chain (preset.Default) does NOT include tracing — callers
// opt in by adding it:
//
//	loop.Middleware = middleware.Chain(
//	    observability.Tracing(observability.TracingConfig{...}),
//	    ...your other middleware
//	)
//
// Spans carry attributes for tool name / risk / call id so traces are
// useful for forensics.
package observability

import (
	"context"
	"fmt"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracingConfig configures the tracing middleware.
type TracingConfig struct {
	// TracerName is the OTel tracer name (typically "agentsdk" or
	// "<tool>.<pipeline>"). Defaults to "agentsdk.runtime".
	TracerName string
	// TracerProvider is the OTel TracerProvider to fetch the Tracer
	// from. Defaults to otel.GetTracerProvider() (the global).
	TracerProvider trace.TracerProvider
}

// Tracing returns a middleware that wraps every effect in a span.
func Tracing(cfg TracingConfig) middleware.Middleware {
	tp := cfg.TracerProvider
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	tracerName := cfg.TracerName
	if tracerName == "" {
		tracerName = "agentsdk.runtime"
	}
	tracer := tp.Tracer(tracerName)
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
			spanCtx, span := tracer.Start(ctx, spanName(eff),
				trace.WithAttributes(spanAttributes(eff)...),
			)
			defer span.End()

			s, in, term, err := next(spanCtx, state, eff)

			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			} else if in != nil && in.ToolResult != nil && !in.ToolResult.OK {
				span.SetStatus(codes.Error, "tool reported not-ok")
			}
			return s, in, term, err
		}
	}
}

// spanName returns the OTel span name for a given effect kind.
func spanName(eff core.Instruction) string {
	switch eff.Kind {
	case core.INSTRUCTION_CALL_MODEL:
		if eff.CallModel != nil {
			return "model." + eff.CallModel.RequestID
		}
	case core.INSTRUCTION_CALL_TOOL:
		if eff.CallTool != nil {
			return "tool." + eff.CallTool.Call.Name
		}
	case core.INSTRUCTION_REQUEST_APPROVAL:
		return "approval.request"
	case core.INSTRUCTION_NOTIFY:
		return "notify"
	case core.INSTRUCTION_DONE:
		return "loop.done"
	}
	return "effect." + string(eff.Kind)
}

// spanAttributes produces the OTel attribute list for an effect.
func spanAttributes(eff core.Instruction) []attribute.KeyValue {
	out := []attribute.KeyValue{
		attribute.String("agentsdk.effect.kind", string(eff.Kind)),
	}
	if eff.CallTool != nil {
		out = append(out,
			attribute.String("agentsdk.tool.name", eff.CallTool.Call.Name),
			attribute.String("agentsdk.tool.call_id", eff.CallTool.Call.ID),
			attribute.String("agentsdk.tool.risk", string(eff.CallTool.Call.Risk)),
		)
	}
	if eff.CallModel != nil {
		out = append(out,
			attribute.String("agentsdk.model.request_id", eff.CallModel.RequestID),
			attribute.Int("agentsdk.model.max_tokens", eff.CallModel.MaxTokens),
		)
	}
	if eff.RequestApproval != nil {
		out = append(out,
			attribute.String("agentsdk.approval.id", eff.RequestApproval.ApprovalID),
			attribute.String("agentsdk.approval.reason", eff.RequestApproval.Reason),
		)
	}
	if eff.Notify != nil {
		out = append(out, attribute.String("agentsdk.notify.level", eff.Notify.Level))
	}
	return out
}

// TracerFromName is a tiny convenience for tests / callers who want to
// resolve a tracer without going through the global provider.
func TracerFromName(tp trace.TracerProvider, name string) trace.Tracer {
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	if name == "" {
		name = "agentsdk.runtime"
	}
	return tp.Tracer(name)
}

// Silence unused import: trace is used by TracingConfig's TracerProvider field.
var _ = fmt.Sprintf
