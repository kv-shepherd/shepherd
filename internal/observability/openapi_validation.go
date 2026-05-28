package observability

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"kv-shepherd.io/shepherd/internal/pkg/httpmethod"
)

const unknownMetricLabel = "unknown"

// OpenAPIValidationRecorder records runtime OpenAPI validation failures.
type OpenAPIValidationRecorder struct {
	failures *prometheus.CounterVec
}

// NewOpenAPIValidationRecorder creates the OpenAPI validation failure recorder.
func NewOpenAPIValidationRecorder() *OpenAPIValidationRecorder {
	return &OpenAPIValidationRecorder{
		failures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "shepherd",
				Subsystem: "openapi",
				Name:      "validation_failures_total",
				Help:      "Total runtime OpenAPI validation failures partitioned by phase, code, method, and normalized route.",
			},
			[]string{"phase", "code", "method", "route"},
		),
	}
}

func (r *OpenAPIValidationRecorder) collector() prometheus.Collector {
	return r.failures
}

// RecordOpenAPIValidationFailure records one setup, request, or response validation failure.
func (r *OpenAPIValidationRecorder) RecordOpenAPIValidationFailure(method, route, phase, code string) {
	if r == nil || r.failures == nil {
		return
	}
	r.failures.WithLabelValues(
		metricLabelOrUnknown(phase),
		metricLabelOrUnknown(code),
		httpmethod.NormalizeForObservability(method),
		metricRouteLabel(route),
	).Inc()
}

func metricLabelOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return unknownMetricLabel
	}
	return value
}

func metricRouteLabel(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return unmatchedRoute
	}
	return route
}
