package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTempoTraceQueryClientSummarizesRoutesAndDependencies(t *testing.T) {
	t.Parallel()

	baseTime := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)
	wantStart := strconv.FormatInt(baseTime.Add(-time.Hour).Unix(), 10)
	wantEnd := strconv.FormatInt(baseTime.Unix(), 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search":
			if got := r.URL.Query().Get("tags"); got != "service.name=shepherd" {
				t.Fatalf("tags = %q, want service.name=shepherd", got)
			}
			if got := r.URL.Query().Get("limit"); got != "2" {
				t.Fatalf("limit = %q, want 2", got)
			}
			assertTempoQueryWindow(t, r, wantStart, wantEnd)
			writeJSON(t, w, map[string]any{
				"traces": []map[string]any{
					{
						"traceID":           "trace-ok",
						"rootTraceName":     "GET /api/v1/vms/:id",
						"startTimeUnixNano": "1000000000",
						"durationMs":        200,
					},
					{
						"traceID":           "trace-error",
						"rootTraceName":     "GET /api/v1/vms/:id",
						"startTimeUnixNano": "2000000000",
						"durationMs":        800,
					},
				},
			})
		case "/api/traces/trace-ok":
			assertTempoQueryWindow(t, r, wantStart, wantEnd)
			writeJSON(t, w, tracePayload([]map[string]any{
				spanPayload("GET /api/v1/vms/:id", "SPAN_KIND_SERVER", 1_000_000_000, 1_200_000_000, map[string]any{
					"http.request.method":       "GET",
					"http.route":                "/api/v1/vms/:id",
					"http.response.status_code": "200",
				}, ""),
				spanPayload("postgresql SELECT tickets", "SPAN_KIND_CLIENT", 1_020_000_000, 1_050_000_000, map[string]any{
					"db.system.name": "postgresql",
				}, ""),
				spanPayload("kubernetes.get.virtualmachines", "SPAN_KIND_CLIENT", 1_060_000_000, 1_120_000_000, map[string]any{
					"rpc.system":        "kubernetes",
					"shepherd.provider": "kubevirt",
				}, ""),
				spanPayload("business.vm.request_create", "SPAN_KIND_INTERNAL", 1_020_000_000, 1_180_000_000, map[string]any{
					"shepherd.business.operation": "vm.request_create",
				}, ""),
			}))
		case "/api/traces/trace-error":
			assertTempoQueryWindow(t, r, wantStart, wantEnd)
			writeJSON(t, w, tracePayload([]map[string]any{
				spanPayload("GET /api/v1/vms/:id", "SPAN_KIND_SERVER", 2_000_000_000, 2_800_000_000, map[string]any{
					"http.request.method":       "GET",
					"http.route":                "/api/v1/vms/:id",
					"http.response.status_code": "500",
				}, "STATUS_CODE_ERROR"),
				spanPayload("postgresql UPDATE tickets", "SPAN_KIND_CLIENT", 2_100_000_000, 2_300_000_000, map[string]any{
					"db.system.name": "postgresql",
				}, "STATUS_CODE_ERROR"),
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewTempoTraceQueryClient(TempoTraceQueryOptions{
		BaseURL:     server.URL,
		ServiceName: "shepherd",
		Limit:       2,
		Lookback:    time.Hour,
		Now:         func() time.Time { return baseTime },
	})
	if err != nil {
		t.Fatalf("NewTempoTraceQueryClient() error = %v", err)
	}

	summary, err := client.TraceSummary(context.Background(), TraceSummaryFilter{Limit: 2})
	if err != nil {
		t.Fatalf("TraceSummary() error = %v", err)
	}

	if summary.Source != "tempo" {
		t.Fatalf("source = %q, want tempo", summary.Source)
	}
	if summary.WindowSeconds != 3600 {
		t.Fatalf("window_seconds = %d, want 3600", summary.WindowSeconds)
	}
	if len(summary.Endpoints) != 1 {
		t.Fatalf("endpoints len = %d, want 1", len(summary.Endpoints))
	}
	endpoint := summary.Endpoints[0]
	if endpoint.Route != "GET /api/v1/vms/:id" {
		t.Fatalf("route = %q", endpoint.Route)
	}
	if endpoint.RequestCount != 2 || endpoint.ErrorCount != 1 {
		t.Fatalf("counts = %d/%d, want 2/1", endpoint.RequestCount, endpoint.ErrorCount)
	}
	if endpoint.P95Ms != 800 || endpoint.AvgMs != 500 || endpoint.MaxMs != 800 {
		t.Fatalf("latency summary = p95 %.2f avg %.2f max %.2f", endpoint.P95Ms, endpoint.AvgMs, endpoint.MaxMs)
	}
	if endpoint.SlowestTraceID != "trace-error" {
		t.Fatalf("slowest trace id = %q, want trace-error", endpoint.SlowestTraceID)
	}
	if len(summary.SlowTraces) != 2 || summary.SlowTraces[0].TraceID != "trace-error" {
		t.Fatalf("slow traces = %#v", summary.SlowTraces)
	}

	wantCategories := map[string]bool{
		"database": false,
		"kubevirt": false,
		"business": false,
	}
	for _, dep := range summary.Dependencies {
		if _, ok := wantCategories[dep.Category]; ok {
			wantCategories[dep.Category] = true
		}
	}
	for category, seen := range wantCategories {
		if !seen {
			t.Fatalf("missing dependency category %s in %#v", category, summary.Dependencies)
		}
	}
}

func TestTempoTraceQueryClientRouteFilter(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search":
			if got := r.URL.Query().Get("tags"); got != "service.name=shepherd http.request.method=POST http.route=/api/v1/vms" {
				t.Fatalf("tags = %q, want route filter tags", got)
			}
			writeJSON(t, w, map[string]any{
				"traces": []map[string]any{{"traceID": "trace-one", "rootTraceName": "GET /api/v1/vms"}},
			})
		case "/api/traces/trace-one":
			writeJSON(t, w, tracePayload([]map[string]any{
				spanPayload("GET /api/v1/vms", "SPAN_KIND_SERVER", 1_000_000_000, 1_100_000_000, map[string]any{
					"http.request.method":       "GET",
					"http.route":                "/api/v1/vms",
					"http.response.status_code": "200",
				}, ""),
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewTempoTraceQueryClient(TempoTraceQueryOptions{BaseURL: server.URL, ServiceName: "shepherd"})
	if err != nil {
		t.Fatalf("NewTempoTraceQueryClient() error = %v", err)
	}
	summary, err := client.TraceSummary(context.Background(), TraceSummaryFilter{Route: "POST /api/v1/vms"})
	if err != nil {
		t.Fatalf("TraceSummary() error = %v", err)
	}
	if len(summary.Endpoints) != 0 {
		t.Fatalf("endpoints = %#v, want empty after route filter", summary.Endpoints)
	}
}

func TestTempoTraceQueryClientRejectsOversizedSearchResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, map[string]any{
			"traces": []map[string]any{{
				"traceID": strings.Repeat("x", 64),
			}},
		})
	}))
	t.Cleanup(server.Close)

	client, err := NewTempoTraceQueryClient(TempoTraceQueryOptions{
		BaseURL:          server.URL,
		MaxResponseBytes: 32,
	})
	if err != nil {
		t.Fatalf("NewTempoTraceQueryClient() error = %v", err)
	}

	_, err = client.TraceSummary(t.Context(), TraceSummaryFilter{})
	if err == nil {
		t.Fatal("TraceSummary() error = nil, want oversized response error")
	}
	if !strings.Contains(err.Error(), "tempo response exceeds 32 bytes") {
		t.Fatalf("TraceSummary() error = %v, want oversized response error", err)
	}
}

func TestTraceAccumulatorSkipsOperationalRoutesByDefault(t *testing.T) {
	t.Parallel()

	acc := newTraceAccumulator("")
	acc.addTrace(tempoSearchTrace{TraceID: "trace-metrics"}, tracePayload([]map[string]any{
		spanPayload("GET /metrics", "SPAN_KIND_SERVER", 1_000_000_000, 1_005_000_000, map[string]any{
			"http.request.method":       "GET",
			"http.route":                "/metrics",
			"http.response.status_code": "200",
		}, ""),
		spanPayload("GET /api/v1/health/ready", "SPAN_KIND_SERVER", 1_010_000_000, 1_015_000_000, map[string]any{
			"http.request.method":       "GET",
			"http.route":                "/api/v1/health/ready",
			"http.response.status_code": "200",
		}, ""),
	}))
	if got := len(acc.endpointSummaries()); got != 0 {
		t.Fatalf("endpoint summaries = %d, want 0", got)
	}
}

func TestTraceAccumulatorRouteFilterMatchesMethodAndPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter string
		route  string
		want   bool
	}{
		{
			name:   "same method and path",
			filter: "GET /api/v1/vms",
			route:  "GET /api/v1/vms",
			want:   true,
		},
		{
			name:   "method mismatch",
			filter: "POST /api/v1/vms",
			route:  "GET /api/v1/vms",
			want:   false,
		},
		{
			name:   "path-only filter matches method route",
			filter: "/api/v1/vms",
			route:  "GET /api/v1/vms",
			want:   true,
		},
		{
			name:   "method filter rejects path-only route",
			filter: "GET /api/v1/vms",
			route:  "/api/v1/vms",
			want:   false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			acc := newTraceAccumulator(tc.filter)
			if got := acc.acceptRoute(tc.route); got != tc.want {
				t.Fatalf("acceptRoute(%q) = %v, want %v", tc.route, got, tc.want)
			}
		})
	}
}

func TestTempoSearchTagsQuotesLogfmtValues(t *testing.T) {
	t.Parallel()

	got := tempoSearchTags("shepherd api", "GET /api/v1/vms/:id")
	want := `service.name="shepherd api" http.request.method=GET http.route=/api/v1/vms/:id`
	if got != want {
		t.Fatalf("tempoSearchTags() = %q, want %q", got, want)
	}
}

func assertTempoQueryWindow(t *testing.T, r *http.Request, wantStart, wantEnd string) {
	t.Helper()
	if got := r.URL.Query().Get("start"); got != wantStart {
		t.Fatalf("start = %q, want %q", got, wantStart)
	}
	if got := r.URL.Query().Get("end"); got != wantEnd {
		t.Fatalf("end = %q, want %q", got, wantEnd)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func tracePayload(spans []map[string]any) map[string]any {
	return map[string]any{
		"batches": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						attrPayload("service.name", "shepherd"),
					},
				},
				"scopeSpans": []map[string]any{
					{"spans": spans},
				},
			},
		},
	}
}

func spanPayload(name, kind string, start, end int64, attrs map[string]any, statusCode string) map[string]any {
	attributes := make([]map[string]any, 0, len(attrs))
	for key, value := range attrs {
		attributes = append(attributes, attrPayload(key, value))
	}
	payload := map[string]any{
		"name":              name,
		"kind":              kind,
		"startTimeUnixNano": strconvFormatInt(start),
		"endTimeUnixNano":   strconvFormatInt(end),
		"attributes":        attributes,
	}
	if statusCode != "" {
		payload["status"] = map[string]any{"code": statusCode}
	}
	return payload
}

func attrPayload(key string, value any) map[string]any {
	return map[string]any{
		"key":   key,
		"value": map[string]any{"stringValue": fmtSprint(value)},
	}
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func fmtSprint(value any) string {
	return fmt.Sprint(value)
}
