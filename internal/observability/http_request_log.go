package observability

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/pkg/httpmethod"
)

const (
	// TraceIDHeader exposes the ingress trace ID for operator/client correlation.
	TraceIDHeader = "X-Shepherd-Trace-ID"

	requestIDHeader           = "X-Request-ID"
	defaultRequestLogMetric   = "/metrics"
	defaultRequestLogRoute    = "unmatched"
	healthRoutePrefix         = "/api/v1/health/"
	requestLogCompletionEvent = "http_request_completed"
)

// HTTPRequestLogOptions configures bounded HTTP request completion logging.
type HTTPRequestLogOptions struct {
	Logger      *zap.Logger
	MetricsPath string
}

// HTTPRequestLogMiddleware emits one bounded request completion log per product request.
func HTTPRequestLogMiddleware(options HTTPRequestLogOptions) gin.HandlerFunc {
	log := options.Logger
	if log == nil {
		log = zap.NewNop()
	}
	metricsPath := strings.TrimSpace(options.MetricsPath)
	if metricsPath == "" {
		metricsPath = defaultRequestLogMetric
	}

	return func(c *gin.Context) {
		start := time.Now()
		spanContext := trace.SpanContextFromContext(c.Request.Context())
		traceID := ""
		spanID := ""
		if spanContext.IsValid() {
			traceID = spanContext.TraceID().String()
			spanID = spanContext.SpanID().String()
			c.Writer.Header().Set(TraceIDHeader, traceID)
		}

		c.Next()

		status := c.Writer.Status()
		path := requestPath(c)
		if shouldSkipHTTPRequestLog(path, status, metricsPath) {
			return
		}

		fields := []zap.Field{
			zap.String("method", httpmethod.NormalizeForObservability(c.Request.Method)),
			zap.String("route", normalizedRequestRoute(c)),
			zap.Int("status", status),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		}
		if requestID := requestIDFromHeaders(c); requestID != "" {
			fields = append(fields, zap.String("request_id", requestID))
		}
		if traceID != "" {
			fields = append(fields,
				zap.String("trace_id", traceID),
				zap.String("span_id", spanID),
			)
		}

		if status >= http.StatusInternalServerError {
			log.Error(requestLogCompletionEvent, fields...)
			return
		}
		log.Info(requestLogCompletionEvent, fields...)
	}
}

func shouldSkipHTTPRequestLog(path string, status int, metricsPath string) bool {
	if status >= http.StatusInternalServerError {
		return false
	}
	return path == metricsPath || strings.HasPrefix(path, healthRoutePrefix)
}

func requestPath(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return c.Request.URL.Path
}

func normalizedRequestRoute(c *gin.Context) string {
	if c == nil {
		return defaultRequestLogRoute
	}
	route := strings.TrimSpace(c.FullPath())
	if route == "" {
		return defaultRequestLogRoute
	}
	return route
}

func requestIDFromHeaders(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if rid := strings.TrimSpace(c.Writer.Header().Get(requestIDHeader)); rid != "" {
		return rid
	}
	return strings.TrimSpace(c.Request.Header.Get(requestIDHeader))
}
