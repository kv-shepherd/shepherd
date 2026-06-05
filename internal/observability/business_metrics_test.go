package observability

import (
	"context"
	"errors"
	"testing"
)

type fakeBusinessMetricsProvider struct {
	stats BusinessMetricsStats
	err   error
}

func (p fakeBusinessMetricsProvider) BusinessMetrics(context.Context) (BusinessMetricsStats, error) {
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
