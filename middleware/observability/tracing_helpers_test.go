package observability_test

import (
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTracerProviderWithRecorder returns a *trace.TracerProvider whose
// spans land in the provided SpanRecorder. The pointer type satisfies
// the trace.TracerProvider interface (defined on the pointer receiver
// of the SDK struct).
func newTracerProviderWithRecorder(rec *tracetest.SpanRecorder) *trace.TracerProvider {
	return trace.NewTracerProvider(trace.WithSpanProcessor(rec))
}