// Package observability owns runtime monitoring primitives.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/observability
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	stdcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"

	"kv-shepherd.io/shepherd/internal/pkg/httpmethod"
)

const unmatchedRoute = "unmatched"

// Metrics owns the Prometheus registry and application collectors.
type Metrics struct {
	registry            *prometheus.Registry
	handler             http.Handler
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	openAPIValidation   *OpenAPIValidationRecorder
}

type metricsOptions struct {
	collectors []prometheus.Collector
}

// Option customizes the metrics registry.
type Option func(*metricsOptions)

// WithCollector registers an additional collector in the isolated registry.
func WithCollector(collector prometheus.Collector) Option {
	return func(options *metricsOptions) {
		if collector == nil {
			return
		}
		options.collectors = append(options.collectors, collector)
	}
}

// NewMetrics creates an isolated Prometheus registry for one application instance.
func NewMetrics(opts ...Option) *Metrics {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		registry:          registry,
		openAPIValidation: NewOpenAPIValidationRecorder(),
		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "shepherd",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total HTTP requests partitioned by method, normalized route, and status code.",
			},
			[]string{"method", "route", "status"},
		),
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "shepherd",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds partitioned by method, normalized route, and status code.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "route", "status"},
		),
	}

	options := metricsOptions{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&options)
	}

	registryCollectors := make([]prometheus.Collector, 0, 6+len(options.collectors))
	registryCollectors = append(registryCollectors,
		stdcollectors.NewGoCollector(),
		stdcollectors.NewProcessCollector(stdcollectors.ProcessCollectorOpts{}),
		stdcollectors.NewBuildInfoCollector(),
		metrics.httpRequestsTotal,
		metrics.httpRequestDuration,
		metrics.openAPIValidation.collector(),
	)
	registryCollectors = append(registryCollectors, options.collectors...)
	registry.MustRegister(registryCollectors...)

	metrics.handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorHandling:     promhttp.ContinueOnError,
		Registry:          registry,
	})
	return metrics
}

// OpenAPIValidationRecorder returns the runtime validation failure recorder.
func (m *Metrics) OpenAPIValidationRecorder() *OpenAPIValidationRecorder {
	if m == nil {
		return nil
	}
	return m.openAPIValidation
}

// Handler returns the Prometheus exposition handler.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}
	return m.handler
}

// Middleware records request volume and latency after downstream handlers complete.
func (m *Metrics) Middleware() gin.HandlerFunc {
	if m == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = unmatchedRoute
		}
		status := strconv.Itoa(c.Writer.Status())
		method := httpmethod.NormalizeForObservability(c.Request.Method)
		elapsed := time.Since(start).Seconds()

		m.httpRequestsTotal.WithLabelValues(method, route, status).Inc()
		m.httpRequestDuration.WithLabelValues(method, route, status).Observe(elapsed)
	}
}

// Gather returns the current metric families for tests and diagnostics.
func (m *Metrics) Gather() ([]*dto.MetricFamily, error) {
	if m == nil {
		return nil, nil
	}
	return m.registry.Gather()
}
