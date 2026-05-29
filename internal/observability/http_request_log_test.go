package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHTTPRequestLogMiddlewareLogsCorrelationFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	core, observed := observer.New(zap.InfoLevel)
	router := gin.New()
	router.Use(HTTPRequestLogMiddleware(HTTPRequestLogOptions{
		Logger:      zap.New(core),
		MetricsPath: "/metrics",
	}))
	router.GET("/api/v1/vms/:id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	traceID := trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	spanID := trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/vms/vm-alpha?debug=hidden", http.NoBody)
	req.Header.Set(requestIDHeader, "req-123")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got := resp.Header().Get(TraceIDHeader); got != traceID.String() {
		t.Fatalf("%s = %q, want %q", TraceIDHeader, got, traceID.String())
	}

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].Message != requestLogCompletionEvent {
		t.Fatalf("message = %q, want %q", entries[0].Message, requestLogCompletionEvent)
	}

	fields := entries[0].ContextMap()
	requireAllowedHTTPRequestLogFields(t, fields)
	want := map[string]string{
		"request_id": "req-123",
		"method":     http.MethodGet,
		"route":      "/api/v1/vms/:id",
		"trace_id":   traceID.String(),
		"span_id":    spanID.String(),
	}
	for key, value := range want {
		if fields[key] != value {
			t.Fatalf("field %s = %v, want %q", key, fields[key], value)
		}
	}
	if fields["status"] != int64(http.StatusNoContent) {
		t.Fatalf("status field = %v, want %d", fields["status"], http.StatusNoContent)
	}
}

func TestHTTPRequestLogMiddlewareSkipsSuccessfulOperationalEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	core, observed := observer.New(zap.InfoLevel)
	router := gin.New()
	router.Use(HTTPRequestLogMiddleware(HTTPRequestLogOptions{
		Logger:      zap.New(core),
		MetricsPath: "/metrics",
	}))
	router.GET("/metrics", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/api/v1/health/live", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for _, path := range []string{"/metrics", "/api/v1/health/live"} {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
	}

	if got := observed.Len(); got != 0 {
		t.Fatalf("log entries = %d, want 0", got)
	}
}

func TestHTTPRequestLogMiddlewareLogsFailedOperationalEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	core, observed := observer.New(zap.InfoLevel)
	router := gin.New()
	router.Use(HTTPRequestLogMiddleware(HTTPRequestLogOptions{
		Logger:      zap.New(core),
		MetricsPath: "/metrics",
	}))
	router.GET("/metrics", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].Level != zap.ErrorLevel {
		t.Fatalf("level = %v, want error", entries[0].Level)
	}
	fields := entries[0].ContextMap()
	requireAllowedHTTPRequestLogFields(t, fields)
	if fields["route"] != "/metrics" {
		t.Fatalf("route field = %v, want /metrics", fields["route"])
	}
	if fields["status"] != int64(http.StatusInternalServerError) {
		t.Fatalf("status field = %v, want %d", fields["status"], http.StatusInternalServerError)
	}
}

func TestHTTPRequestLogMiddlewareNormalizesUnknownHTTPMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	core, observed := observer.New(zap.InfoLevel)
	router := gin.New()
	router.Use(HTTPRequestLogMiddleware(HTTPRequestLogOptions{
		Logger:      zap.New(core),
		MetricsPath: "/metrics",
	}))

	req := httptest.NewRequest("BREW", "/unknown/path", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	requireAllowedHTTPRequestLogFields(t, fields)
	if fields["method"] != "_OTHER" {
		t.Fatalf("method field = %v, want _OTHER", fields["method"])
	}
	if fields["route"] != unmatchedRoute {
		t.Fatalf("route field = %v, want %s", fields["route"], unmatchedRoute)
	}
}

func requireAllowedHTTPRequestLogFields(t *testing.T, fields map[string]interface{}) {
	t.Helper()
	allowed := map[string]struct{}{
		"request_id":  {},
		"trace_id":    {},
		"span_id":     {},
		"method":      {},
		"route":       {},
		"status":      {},
		"duration_ms": {},
	}
	for key := range fields {
		if _, ok := allowed[key]; !ok {
			t.Fatalf("unexpected HTTP request log field %q in %v", key, fields)
		}
	}
}
