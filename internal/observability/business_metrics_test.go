package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

type fakeBusinessMetricsProvider struct {
	stats   BusinessMetricsStats
	err     error
	inspect func(context.Context)
}

func (p fakeBusinessMetricsProvider) BusinessMetrics(ctx context.Context) (BusinessMetricsStats, error) {
	if p.inspect != nil {
		p.inspect(ctx)
	}
	if p.err != nil {
		return BusinessMetricsStats{}, p.err
	}
	return p.stats, nil
}

func TestBusinessMetricsCollectorEmitsApprovalAndAuditMetrics(t *testing.T) {
	metrics := NewMetrics(WithBusinessMetricsProvider(fakeBusinessMetricsProvider{
		stats: BusinessMetricsStats{
			ApprovalTickets: []BusinessApprovalTicketCount{
				{Status: "PENDING", OperationType: "CREATE", Count: 4},
				{Status: "FAILED", OperationType: "POWER", Count: 2},
			},
			ApprovalPendingAges: []BusinessApprovalPendingAge{
				{OperationType: "CREATE", AgeSeconds: 25 * 3600},
			},
			BatchApprovalTickets: []BusinessBatchApprovalTicketCount{
				{Status: "PENDING_APPROVAL", BatchType: "BATCH_CREATE", Count: 1},
				{Status: "FAILED", BatchType: "BATCH_POWER", Count: 1},
			},
			BatchApprovalPendingAges: []BusinessBatchApprovalPendingAge{
				{BatchType: "BATCH_CREATE", AgeSeconds: 12 * 3600},
			},
			BatchApprovalFailedChildren: []BusinessBatchApprovalFailedChildCount{
				{BatchType: "BATCH_POWER", Count: 3},
			},
			ApprovalAuditActions: []BusinessApprovalAuditActionCount{
				{Action: "approval.approved", Count: 6},
				{Action: "approval.validation_failed", Count: 2},
			},
			ApprovalFailureAuditActions: []BusinessApprovalAuditActionCount{
				{Action: "approval.validation_failed", Count: 2},
			},
		},
	}))

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	success := findMetric(t, families, "shepherd_business_metrics_scrape_success", nil)
	if got := success.GetGauge().GetValue(); got != 1 {
		t.Fatalf("scrape success = %v, want 1", got)
	}

	pending := findMetric(t, families, "shepherd_business_approval_tickets", map[string]string{
		"status":         "PENDING",
		"operation_type": "CREATE",
	})
	if got := pending.GetGauge().GetValue(); got != 4 {
		t.Fatalf("pending approval tickets = %v, want 4", got)
	}

	failed := findMetric(t, families, "shepherd_business_approval_tickets", map[string]string{
		"status":         "FAILED",
		"operation_type": "POWER",
	})
	if got := failed.GetGauge().GetValue(); got != 2 {
		t.Fatalf("failed approval tickets = %v, want 2", got)
	}

	oldest := findMetric(t, families, "shepherd_business_approval_pending_oldest_age_seconds", map[string]string{
		"operation_type": "CREATE",
	})
	if got := oldest.GetGauge().GetValue(); got != 25*3600 {
		t.Fatalf("oldest approval age = %v, want %v", got, 25*3600)
	}

	batchPending := findMetric(t, families, "shepherd_business_batch_approval_tickets", map[string]string{
		"status":     "PENDING_APPROVAL",
		"batch_type": "BATCH_CREATE",
	})
	if got := batchPending.GetGauge().GetValue(); got != 1 {
		t.Fatalf("pending batch approvals = %v, want 1", got)
	}

	batchAge := findMetric(t, families, "shepherd_business_batch_approval_pending_oldest_age_seconds", map[string]string{
		"batch_type": "BATCH_CREATE",
	})
	if got := batchAge.GetGauge().GetValue(); got != 12*3600 {
		t.Fatalf("oldest batch approval age = %v, want %v", got, 12*3600)
	}

	failedChildren := findMetric(t, families, "shepherd_business_batch_approval_failed_children", map[string]string{
		"batch_type": "BATCH_POWER",
	})
	if got := failedChildren.GetGauge().GetValue(); got != 3 {
		t.Fatalf("failed batch children = %v, want 3", got)
	}

	auditAction := findMetric(t, families, "shepherd_business_approval_audit_actions_recent", map[string]string{
		"action": "approval.approved",
	})
	if got := auditAction.GetGauge().GetValue(); got != 6 {
		t.Fatalf("recent approval audit actions = %v, want 6", got)
	}

	failureAction := findMetric(t, families, "shepherd_business_approval_failure_audit_actions_recent", map[string]string{
		"action": "approval.validation_failed",
	})
	if got := failureAction.GetGauge().GetValue(); got != 2 {
		t.Fatalf("recent approval failure audit actions = %v, want 2", got)
	}
}

func TestBusinessMetricsCollectorReportsScrapeFailure(t *testing.T) {
	metrics := NewMetrics(WithBusinessMetricsProvider(fakeBusinessMetricsProvider{
		err: errors.New("database unavailable"),
	}))

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	success := findMetric(t, families, "shepherd_business_metrics_scrape_success", nil)
	if got := success.GetGauge().GetValue(); got != 0 {
		t.Fatalf("scrape success = %v, want 0", got)
	}
}

func TestBusinessMetricsCollectorPassesBoundedScrapeContext(t *testing.T) {
	seenDeadline := false
	metrics := NewMetrics(WithBusinessMetricsProvider(fakeBusinessMetricsProvider{
		inspect: func(ctx context.Context) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("BusinessMetrics context missing deadline")
			}
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > defaultCollectorScrapeTimeout {
				t.Fatalf("BusinessMetrics context remaining deadline = %s, want within %s", remaining, defaultCollectorScrapeTimeout)
			}
			seenDeadline = true
		},
	}))

	if _, err := metrics.Gather(); err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if !seenDeadline {
		t.Fatal("BusinessMetrics provider was not called")
	}
}

func TestBusinessMetricsCollectorNormalizesAuditActionLabels(t *testing.T) {
	metrics := NewMetrics(WithBusinessMetricsProvider(fakeBusinessMetricsProvider{
		stats: BusinessMetricsStats{
			ApprovalAuditActions: []BusinessApprovalAuditActionCount{
				{Action: "approval.approved", Count: 5},
				{Action: "approval.future_dynamic_action", Count: 6},
				{Action: " external_approval.callback.accepted ", Count: 7},
			},
			ApprovalFailureAuditActions: []BusinessApprovalAuditActionCount{
				{Action: "approval.validation_failed", Count: 2},
				{Action: "approval.future_dynamic_failure", Count: 3},
				{Action: " external_approval.callback.failed ", Count: 4},
			},
		},
	}))

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	knownRecent := findMetric(t, families, "shepherd_business_approval_audit_actions_recent", map[string]string{
		"action": "approval.approved",
	})
	if got := knownRecent.GetGauge().GetValue(); got != 5 {
		t.Fatalf("known recent action count = %v, want 5", got)
	}

	otherRecent := findMetric(t, families, "shepherd_business_approval_audit_actions_recent", map[string]string{
		"action": BusinessApprovalAuditOtherAction,
	})
	if got := otherRecent.GetGauge().GetValue(); got != 13 {
		t.Fatalf("other recent action count = %v, want 13", got)
	}

	if metricExists(families, "shepherd_business_approval_audit_actions_recent", map[string]string{
		"action": "approval.future_dynamic_action",
	}) {
		t.Fatal("unexpected raw future recent action label was emitted")
	}
	if metricExists(families, "shepherd_business_approval_audit_actions_recent", map[string]string{
		"action": "external_approval.callback.accepted",
	}) {
		t.Fatal("unexpected raw external approval recent action label was emitted")
	}

	known := findMetric(t, families, "shepherd_business_approval_failure_audit_actions_recent", map[string]string{
		"action": "approval.validation_failed",
	})
	if got := known.GetGauge().GetValue(); got != 2 {
		t.Fatalf("known failure action count = %v, want 2", got)
	}

	other := findMetric(t, families, "shepherd_business_approval_failure_audit_actions_recent", map[string]string{
		"action": BusinessApprovalAuditOtherAction,
	})
	if got := other.GetGauge().GetValue(); got != 7 {
		t.Fatalf("other failure action count = %v, want 7", got)
	}

	if metricExists(families, "shepherd_business_approval_failure_audit_actions_recent", map[string]string{
		"action": "approval.future_dynamic_failure",
	}) {
		t.Fatal("unexpected raw future action label was emitted")
	}
	if metricExists(families, "shepherd_business_approval_failure_audit_actions_recent", map[string]string{
		"action": "external_approval.callback.failed",
	}) {
		t.Fatal("unexpected raw external approval action label was emitted")
	}
}

func TestNormalizeBusinessApprovalAuditActionCountsAggregatesOther(t *testing.T) {
	got := normalizeBusinessApprovalAuditActionCounts([]BusinessApprovalAuditActionCount{
		{Action: "approval.batch_rejected", Count: 1},
		{Action: "approval.dynamic_one", Count: 2},
		{Action: "", Count: 3},
	})

	want := []BusinessApprovalAuditActionCount{
		{Action: "approval.batch_rejected", Count: 1},
		{Action: BusinessApprovalAuditOtherAction, Count: 5},
	}
	if len(got) != len(want) {
		t.Fatalf("normalized len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalized[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func metricExists(families []*dto.MetricFamily, name string, labels map[string]string) bool {
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricHasLabels(metric, labels) {
				return true
			}
		}
	}
	return false
}
