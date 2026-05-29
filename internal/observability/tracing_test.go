package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestNewTracingDisabledReturnsNil(t *testing.T) {
	tracing, err := NewTracing(context.Background(), TracingOptions{Enabled: false})
	if err != nil {
		t.Fatalf("NewTracing() error = %v", err)
	}
	if tracing != nil {
		t.Fatalf("NewTracing() = %#v, want nil", tracing)
	}
}

func TestTracingMiddlewareExtractsTraceParent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tracing, err := NewTracing(context.Background(), TracingOptions{
		Enabled:         true,
		ServiceName:     "shepherd-test",
		Exporter:        TracingExporterStdout,
		SampleRatio:     1,
		ShutdownTimeout: 0,
	})
	if err != nil {
		t.Fatalf("NewTracing() error = %v", err)
	}
	t.Cleanup(func() {
		if err := tracing.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	})

	var gotTraceID string
	router := gin.New()
	router.Use(tracing.Middleware())
	router.GET("/api/v1/health/live", func(c *gin.Context) {
		gotTraceID = oteltrace.SpanContextFromContext(c.Request.Context()).TraceID().String()
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", http.NoBody)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if gotTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id = %q, want propagated trace id", gotTraceID)
	}
}

func TestTracingMiddlewareUsesNormalizedRouteAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	tracing := &Tracing{
		serviceName: "shepherd-test",
		provider:    provider,
		tracer:      provider.Tracer(tracingInstrumentationName),
	}
	t.Cleanup(func() {
		if err := tracing.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	})

	router := gin.New()
	router.Use(tracing.Middleware())
	router.GET("/api/v1/vms/:id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vms/vm-alpha?debug=hidden", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "GET /api/v1/vms/:id" {
		t.Fatalf("span name = %q, want normalized route span name", span.Name())
	}

	attributes := map[string]string{}
	for _, attr := range span.Attributes() {
		attributes[string(attr.Key)] = attr.Value.AsString()
		if strings.Contains(attr.Value.AsString(), "vm-alpha") || strings.Contains(attr.Value.AsString(), "debug=hidden") {
			t.Fatalf("span attribute %s leaked raw request data: %q", attr.Key, attr.Value.AsString())
		}
	}
	if attributes["http.request.method"] != http.MethodGet {
		t.Fatalf("http.request.method = %q, want GET", attributes["http.request.method"])
	}
	if attributes["http.route"] != "/api/v1/vms/:id" {
		t.Fatalf("http.route = %q, want normalized route", attributes["http.route"])
	}
	if _, ok := attributes["url.path"]; ok {
		t.Fatalf("url.path attribute must not be emitted: %v", attributes)
	}
	if _, ok := attributes["url.full"]; ok {
		t.Fatalf("url.full attribute must not be emitted: %v", attributes)
	}
}

func TestTracingMiddlewareNormalizesUnknownHTTPMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	tracing := &Tracing{
		serviceName: "shepherd-test",
		provider:    provider,
		tracer:      provider.Tracer(tracingInstrumentationName),
	}
	t.Cleanup(func() {
		if err := tracing.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	})

	router := gin.New()
	router.Use(tracing.Middleware())

	req := httptest.NewRequest("BREW", "/unknown/path", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "_OTHER unmatched" {
		t.Fatalf("span name = %q, want _OTHER unmatched", span.Name())
	}
	attributes := map[string]string{}
	for _, attr := range span.Attributes() {
		attributes[string(attr.Key)] = attr.Value.AsString()
	}
	if attributes["http.request.method"] != "_OTHER" {
		t.Fatalf("http.request.method = %q, want _OTHER", attributes["http.request.method"])
	}
	if attributes["http.route"] != unmatchedRoute {
		t.Fatalf("http.route = %q, want %s", attributes["http.route"], unmatchedRoute)
	}
}

func TestNewTracingRejectsInvalidOptions(t *testing.T) {
	testCases := []struct {
		name    string
		options TracingOptions
	}{
		{
			name: "missing service name",
			options: TracingOptions{
				Enabled:     true,
				Exporter:    TracingExporterStdout,
				SampleRatio: 1,
			},
		},
		{
			name: "invalid sample ratio",
			options: TracingOptions{
				Enabled:     true,
				ServiceName: "shepherd-test",
				Exporter:    TracingExporterStdout,
				SampleRatio: 1.1,
			},
		},
		{
			name: "invalid exporter",
			options: TracingOptions{
				Enabled:     true,
				ServiceName: "shepherd-test",
				Exporter:    "invalid",
				SampleRatio: 1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tracing, err := NewTracing(context.Background(), tc.options)
			if err == nil {
				t.Fatal("NewTracing() error = nil, want error")
			}
			if tracing != nil {
				t.Fatalf("NewTracing() = %#v, want nil on error", tracing)
			}
		})
	}
}
