package provider

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"kv-shepherd.io/shepherd/internal/observability"
)

func startKubeClientSpan(
	ctx context.Context,
	operation string,
	resource string,
	namespace string,
	attrs ...attribute.KeyValue,
) (context.Context, oteltrace.Span) {
	operation = sanitizeTraceValue(operation)
	resource = sanitizeTraceValue(resource)
	namespace = strings.TrimSpace(namespace)
	baseAttrs := make([]attribute.KeyValue, 0, 6+len(attrs))
	baseAttrs = append(baseAttrs,
		attribute.String("shepherd.provider", "kubevirt"),
		attribute.String("rpc.system", "kubernetes"),
		attribute.String("rpc.service", "kube-apiserver"),
		attribute.String("rpc.method", operation),
		attribute.String("k8s.resource.resource", resource),
	)
	if namespace != "" {
		baseAttrs = append(baseAttrs, attribute.String("k8s.namespace.name", namespace))
	}
	baseAttrs = append(baseAttrs, attrs...)
	return observability.StartSpanWithOptions(
		ctx,
		"kubernetes."+operation+"."+resource,
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(baseAttrs...),
	)
}

func endTraceSpan(span oteltrace.Span, err error) {
	observability.RecordSpanError(span, err)
	span.End()
}

func sanitizeTraceValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
		if b.Len() >= 80 {
			break
		}
	}
	return b.String()
}
