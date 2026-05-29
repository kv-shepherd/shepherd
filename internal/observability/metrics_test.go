package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricsMiddlewareRecordsNormalizedRouteStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	metrics := NewMetrics()
	router := gin.New()
	router.Use(metrics.Middleware())
	router.GET("/api/v1/widgets/:id", func(c *gin.Context) {
		c.String(http.StatusCreated, "created")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/widget-123", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("response status = %d, want %d", resp.Code, http.StatusCreated)
	}

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	counter := findMetric(t, families, "shepherd_http_requests_total", map[string]string{
		"method": http.MethodGet,
		"route":  "/api/v1/widgets/:id",
		"status": "201",
	})
	if got := counter.GetCounter().GetValue(); got != 1 {
		t.Fatalf("http request counter = %v, want 1", got)
	}
	histogram := findMetric(t, families, "shepherd_http_request_duration_seconds", map[string]string{
		"method": http.MethodGet,
		"route":  "/api/v1/widgets/:id",
		"status": "201",
	})
	if got := histogram.GetHistogram().GetSampleCount(); got != 1 {
		t.Fatalf("http request histogram sample count = %d, want 1", got)
	}
}

func TestMetricsMiddlewareUsesUnmatchedFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	metrics := NewMetrics()
	router := gin.New()
	router.Use(metrics.Middleware())

	req := httptest.NewRequest(http.MethodGet, "/unknown/path", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("response status = %d, want %d", resp.Code, http.StatusNotFound)
	}

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	counter := findMetric(t, families, "shepherd_http_requests_total", map[string]string{
		"method": http.MethodGet,
		"route":  unmatchedRoute,
		"status": "404",
	})
	if got := counter.GetCounter().GetValue(); got != 1 {
		t.Fatalf("unmatched route counter = %v, want 1", got)
	}
}

func TestMetricsMiddlewareNormalizesUnknownHTTPMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	metrics := NewMetrics()
	router := gin.New()
	router.Use(metrics.Middleware())

	req := httptest.NewRequest("BREW", "/unknown/path", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("response status = %d, want %d", resp.Code, http.StatusNotFound)
	}

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	counter := findMetric(t, families, "shepherd_http_requests_total", map[string]string{
		"method": "_OTHER",
		"route":  unmatchedRoute,
		"status": "404",
	})
	if got := counter.GetCounter().GetValue(); got != 1 {
		t.Fatalf("unknown method counter = %v, want 1", got)
	}
}

func TestMetricsHandlerExposesPrometheusText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	metrics := NewMetrics()
	router := gin.New()
	router.Use(metrics.Middleware())
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/metrics", gin.WrapH(metrics.Handler()))

	warmupReq := httptest.NewRequest(http.MethodGet, "/ping", http.NoBody)
	warmupResp := httptest.NewRecorder()
	router.ServeHTTP(warmupResp, warmupReq)
	if warmupResp.Code != http.StatusNoContent {
		t.Fatalf("warmup status = %d, want %d", warmupResp.Code, http.StatusNoContent)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	body := resp.Body.String()
	for _, fragment := range []string{
		"shepherd_http_requests_total",
		"go_goroutines",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("metrics response missing %q; body prefix=%q", fragment, body[:min(len(body), 512)])
		}
	}
}

func TestOpenAPIValidationRecorderRecordsFailureCounter(t *testing.T) {
	metrics := NewMetrics()
	metrics.OpenAPIValidationRecorder().RecordOpenAPIValidationFailure(
		http.MethodPost,
		"/api/v1/vms/request",
		"request",
		"OPENAPI_REQUEST_INVALID",
	)

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	counter := findMetric(t, families, "shepherd_openapi_validation_failures_total", map[string]string{
		"phase":  "request",
		"code":   "OPENAPI_REQUEST_INVALID",
		"method": http.MethodPost,
		"route":  "/api/v1/vms/request",
	})
	if got := counter.GetCounter().GetValue(); got != 1 {
		t.Fatalf("openapi validation failure counter = %v, want 1", got)
	}
}

func TestOpenAPIValidationRecorderNormalizesEmptyRoute(t *testing.T) {
	metrics := NewMetrics()
	metrics.OpenAPIValidationRecorder().RecordOpenAPIValidationFailure(http.MethodGet, "", "setup", "OPENAPI_VALIDATOR_UNAVAILABLE")

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	counter := findMetric(t, families, "shepherd_openapi_validation_failures_total", map[string]string{
		"phase":  "setup",
		"code":   "OPENAPI_VALIDATOR_UNAVAILABLE",
		"method": http.MethodGet,
		"route":  unmatchedRoute,
	})
	if got := counter.GetCounter().GetValue(); got != 1 {
		t.Fatalf("openapi validation failure counter = %v, want 1", got)
	}
}

func TestOpenAPIValidationRecorderNormalizesUnknownHTTPMethod(t *testing.T) {
	metrics := NewMetrics()
	metrics.OpenAPIValidationRecorder().RecordOpenAPIValidationFailure("BREW", "/api/v1/vms/request", "request", "OPENAPI_REQUEST_INVALID")

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	counter := findMetric(t, families, "shepherd_openapi_validation_failures_total", map[string]string{
		"phase":  "request",
		"code":   "OPENAPI_REQUEST_INVALID",
		"method": "_OTHER",
		"route":  "/api/v1/vms/request",
	})
	if got := counter.GetCounter().GetValue(); got != 1 {
		t.Fatalf("openapi validation failure counter = %v, want 1", got)
	}
}

func findMetric(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) *dto.Metric {
	t.Helper()
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricHasLabels(metric, labels) {
				return metric
			}
		}
	}
	t.Fatalf("metric %s with labels %v not found", name, labels)
	return nil
}

func metricHasLabels(metric *dto.Metric, labels map[string]string) bool {
	actual := make(map[string]string, len(metric.GetLabel()))
	for _, label := range metric.GetLabel() {
		actual[label.GetName()] = label.GetValue()
	}
	for key, want := range labels {
		if actual[key] != want {
			return false
		}
	}
	return true
}
