package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const shepherdTracingInstrumentationName = "kv-shepherd.io/shepherd"

// StartSpan starts an internal Shepherd span using the process-wide tracer provider.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, oteltrace.Span) {
	return StartSpanWithOptions(ctx, name, oteltrace.WithAttributes(attrs...))
}

// StartSpanWithOptions starts a Shepherd span with explicit OpenTelemetry options.
func StartSpanWithOptions(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return otel.Tracer(shepherdTracingInstrumentationName).Start(
		ctx,
		name,
		opts...,
	)
}

// RecordSpanError marks the span failed when err is non-nil.
func RecordSpanError(span oteltrace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
