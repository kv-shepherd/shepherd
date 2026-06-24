package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/observability"
)

type fakeTraceSummaryProvider struct {
	summary observability.TraceSummary
	err     error
	filter  observability.TraceSummaryFilter
}

func (p *fakeTraceSummaryProvider) TraceSummary(_ context.Context, filter observability.TraceSummaryFilter) (observability.TraceSummary, error) {
	p.filter = filter
	if p.err != nil {
		return observability.TraceSummary{}, p.err
	}
	return p.summary, nil
}

type fakeBusinessMetricsProvider struct {
	stats observability.BusinessMetricsStats
	err   error
}

func (p fakeBusinessMetricsProvider) BusinessMetrics(context.Context) (observability.BusinessMetricsStats, error) {
	if p.err != nil {
		return observability.BusinessMetricsStats{}, p.err
	}
	return p.stats, nil
}

func TestGetAdminTraceSummaryReturnsAggregates(t *testing.T) {
	t.Parallel()

	generatedAt := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)
	provider := &fakeTraceSummaryProvider{
		summary: observability.TraceSummary{
			GeneratedAt:   generatedAt,
			Source:        "tempo",
			Status:        "ok",
			WindowSeconds: 900,
			Endpoints: []observability.TraceEndpointSummary{
				{
					Route:          "GET /api/v1/vms/:id",
					RequestCount:   10,
					ErrorCount:     1,
					ErrorRate:      0.1,
					P95Ms:          120.5,
					AvgMs:          42.3,
					MaxMs:          300.1,
					SlowestTraceID: "trace-1",
				},
			},
			SlowTraces: []observability.TraceSample{
				{
					TraceID:    "trace-1",
					RootName:   "GET /api/v1/vms/:id",
					Route:      "GET /api/v1/vms/:id",
					DurationMs: 300.1,
					StatusCode: 500,
					Error:      true,
					StartedAt:  generatedAt.Add(-time.Minute),
				},
			},
			Dependencies: []observability.TraceSpanGroupSummary{
				{Category: "kubevirt", Name: "kubernetes.get.virtualmachines", SpanCount: 4, ErrorCount: 1, P95Ms: 80, MaxMs: 120},
			},
		},
	}
	srv := NewServer(ServerDeps{TraceSummaryProvider: provider})
	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/observability/traces?lookback_minutes=15&limit=25&route=GET%20/api/v1/vms/:id",
		"",
		"admin-1",
		[]string{"observability:read"},
	)

	srv.GetAdminTraceSummary(c, generated.GetAdminTraceSummaryParams{
		LookbackMinutes: 15,
		Limit:           25,
		Route:           "GET /api/v1/vms/:id",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if provider.filter.Lookback != 15*time.Minute || provider.filter.Limit != 25 || provider.filter.Route != "GET /api/v1/vms/:id" {
		t.Fatalf("filter = %#v", provider.filter)
	}
	var resp generated.AdminObservabilityTraceSummary
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Source != "tempo" || resp.Status != "ok" || resp.WindowSeconds != 900 {
		t.Fatalf("summary = %#v", resp)
	}
	if len(resp.Endpoints) != 1 || resp.Endpoints[0].SlowestTraceId != "trace-1" {
		t.Fatalf("endpoints = %#v", resp.Endpoints)
	}
	if len(resp.Dependencies) != 1 || resp.Dependencies[0].Category != generated.Kubevirt {
		t.Fatalf("dependencies = %#v", resp.Dependencies)
	}
}

func TestGetAdminTraceSummaryReturnsServiceUnavailableWhenUnconfigured(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/observability/traces",
		"",
		"admin-1",
		[]string{"observability:read"},
	)

	srv.GetAdminTraceSummary(c, generated.GetAdminTraceSummaryParams{})

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	var resp generated.Error
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != "TRACE_QUERY_UNAVAILABLE" {
		t.Fatalf("code = %q", resp.Code)
	}
}

func TestGetAdminTraceSummaryRejectsOutOfRangeQuery(t *testing.T) {
	t.Parallel()

	provider := &fakeTraceSummaryProvider{}
	tests := []struct {
		name   string
		params generated.GetAdminTraceSummaryParams
	}{
		{
			name:   "lookback too small",
			params: generated.GetAdminTraceSummaryParams{LookbackMinutes: 1, Limit: 100},
		},
		{
			name:   "lookback too large",
			params: generated.GetAdminTraceSummaryParams{LookbackMinutes: 1441, Limit: 100},
		},
		{
			name:   "limit too large",
			params: generated.GetAdminTraceSummaryParams{LookbackMinutes: 60, Limit: 501},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := NewServer(ServerDeps{TraceSummaryProvider: provider})
			c, w := newAuthedGinContext(
				t,
				http.MethodGet,
				"/admin/observability/traces",
				"",
				"admin-1",
				[]string{"observability:read"},
			)

			srv.GetAdminTraceSummary(c, tc.params)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			var resp generated.Error
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.Code != "INVALID_REQUEST" {
				t.Fatalf("code = %q, want INVALID_REQUEST", resp.Code)
			}
		})
	}
}

func TestGetAdminAuditSignalsReturnsBusinessSignals(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{
		BusinessMetrics: fakeBusinessMetricsProvider{
			stats: observability.BusinessMetricsStats{
				ApprovalTickets: []observability.BusinessApprovalTicketCount{
					{Status: "PENDING", OperationType: "CREATE", Count: 2},
				},
				ApprovalPendingAges: []observability.BusinessApprovalPendingAge{
					{OperationType: "CREATE", AgeSeconds: 7200},
				},
				BatchApprovalFailedChildren: []observability.BusinessBatchApprovalFailedChildCount{
					{BatchType: "POWER", Count: 1},
				},
				ApprovalFailureAuditActions: []observability.BusinessApprovalAuditActionCount{
					{Action: "approval.validation_failed", Count: 3},
					{Action: "approval.future_dynamic_failure", Count: 4},
					{Action: "external_approval.callback.failed", Count: 5},
				},
			},
		},
	})
	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/observability/audit-signals",
		"",
		"admin-1",
		[]string{"observability:read"},
	)

	srv.GetAdminAuditSignals(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp generated.AdminObservabilityAuditSignalSummary
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" || resp.WindowSeconds != int64(time.Hour.Seconds()) {
		t.Fatalf("summary = %#v", resp)
	}
	if len(resp.ApprovalTickets) != 1 || resp.ApprovalTickets[0].Count != 2 {
		t.Fatalf("approval tickets = %#v", resp.ApprovalTickets)
	}
	if len(resp.ApprovalFailureAuditActions) != 2 {
		t.Fatalf("failure actions = %#v", resp.ApprovalFailureAuditActions)
	}
	failureActions := make(map[string]float64, len(resp.ApprovalFailureAuditActions))
	for _, item := range resp.ApprovalFailureAuditActions {
		failureActions[item.Action] = item.Count
	}
	if failureActions["approval.validation_failed"] != 3 {
		t.Fatalf("validation failure action count = %v, want 3", failureActions["approval.validation_failed"])
	}
	if failureActions[observability.BusinessApprovalAuditOtherAction] != 9 {
		t.Fatalf("other failure action count = %v, want 9", failureActions[observability.BusinessApprovalAuditOtherAction])
	}
}
