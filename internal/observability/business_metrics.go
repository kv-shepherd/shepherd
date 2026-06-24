package observability

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	businessApprovalAuditRecentWindow = time.Hour

	// BusinessApprovalAuditOtherAction aggregates non-allowlisted approval audit actions.
	BusinessApprovalAuditOtherAction = "approval.other"

	businessApprovalTicketsByStatusQuery = `
SELECT status::text, operation_type::text, count(*)::float8
FROM tickets
GROUP BY status, operation_type
ORDER BY status, operation_type
`

	businessApprovalPendingOldestAgeQuery = `
SELECT operation_type::text, GREATEST(EXTRACT(EPOCH FROM now() - MIN(created_at)), 0)::float8
FROM tickets
WHERE status = 'PENDING'
GROUP BY operation_type
ORDER BY operation_type
`

	businessBatchApprovalTicketsByStatusQuery = `
SELECT status::text, batch_type::text, count(*)::float8
FROM batch_tickets
GROUP BY status, batch_type
ORDER BY status, batch_type
`

	businessBatchApprovalPendingOldestAgeQuery = `
SELECT batch_type::text, GREATEST(EXTRACT(EPOCH FROM now() - MIN(created_at)), 0)::float8
FROM batch_tickets
WHERE status = 'PENDING_APPROVAL'
GROUP BY batch_type
ORDER BY batch_type
`

	businessBatchApprovalFailedChildrenQuery = `
SELECT batch_type::text, COALESCE(SUM(failed_count), 0)::float8
FROM batch_tickets
GROUP BY batch_type
ORDER BY batch_type
`

	businessApprovalAuditActionsRecentQuery = `
SELECT action, count(*)::float8
FROM audit_logs
WHERE created_at >= now() - ($1::int * interval '1 second')
  AND (action LIKE 'approval.%' OR action LIKE 'external_approval.%')
GROUP BY action
ORDER BY action
`

	businessApprovalFailureAuditActionsRecentQuery = `
SELECT action, count(*)::float8
FROM audit_logs
WHERE created_at >= now() - ($1::int * interval '1 second')
  AND (action LIKE 'approval.%' OR action LIKE 'external_approval.%')
  AND (
    action LIKE '%failed%'
    OR action LIKE '%failure%'
    OR action LIKE '%error%'
  )
GROUP BY action
ORDER BY action
`
)

var businessApprovalAuditActionAllowlist = map[string]struct{}{
	"approval.approved":            {},
	"approval.rejected":            {},
	"approval.validation_failed":   {},
	"approval.power_approved":      {},
	"approval.delete_approved":     {},
	"approval.vnc_access_approved": {},
	"approval.cancelled":           {},
	"approval.batch_approved":      {},
	"approval.batch_rejected":      {},
	"approval.batch_cancelled":     {},
}

// BusinessApprovalTicketCount is one aggregate approval ticket count.
type BusinessApprovalTicketCount struct {
	Status        string
	OperationType string
	Count         float64
}

// BusinessApprovalPendingAge is the oldest pending approval age for one operation type.
type BusinessApprovalPendingAge struct {
	OperationType string
	AgeSeconds    float64
}

// BusinessBatchApprovalTicketCount is one aggregate batch approval count.
type BusinessBatchApprovalTicketCount struct {
	Status    string
	BatchType string
	Count     float64
}

// BusinessBatchApprovalPendingAge is the oldest pending batch approval age for one batch type.
type BusinessBatchApprovalPendingAge struct {
	BatchType  string
	AgeSeconds float64
}

// BusinessBatchApprovalFailedChildCount is one aggregate failed child count by batch type.
type BusinessBatchApprovalFailedChildCount struct {
	BatchType string
	Count     float64
}

// BusinessApprovalAuditActionCount is one aggregate recent approval audit action count.
type BusinessApprovalAuditActionCount struct {
	Action string
	Count  float64
}

// BusinessMetricsStats is the scrape-time business observability snapshot.
type BusinessMetricsStats struct {
	ApprovalTickets             []BusinessApprovalTicketCount
	ApprovalPendingAges         []BusinessApprovalPendingAge
	BatchApprovalTickets        []BusinessBatchApprovalTicketCount
	BatchApprovalPendingAges    []BusinessBatchApprovalPendingAge
	BatchApprovalFailedChildren []BusinessBatchApprovalFailedChildCount
	ApprovalAuditActions        []BusinessApprovalAuditActionCount
	ApprovalFailureAuditActions []BusinessApprovalAuditActionCount
}

// BusinessMetricsProvider provides low-cardinality business metrics.
type BusinessMetricsProvider interface {
	BusinessMetrics(context.Context) (BusinessMetricsStats, error)
}

type pgxBusinessMetricsProvider struct {
	queryer           pgxQueryer
	timeout           time.Duration
	recentAuditWindow time.Duration
}

// NewPGXBusinessMetricsProvider reads business observability aggregates through pgx.
func NewPGXBusinessMetricsProvider(pool *pgxpool.Pool, timeout time.Duration) BusinessMetricsProvider {
	if pool == nil {
		return &pgxBusinessMetricsProvider{
			timeout:           timeout,
			recentAuditWindow: businessApprovalAuditRecentWindow,
		}
	}
	return &pgxBusinessMetricsProvider{
		queryer:           pool,
		timeout:           timeout,
		recentAuditWindow: businessApprovalAuditRecentWindow,
	}
}

func (p *pgxBusinessMetricsProvider) BusinessMetrics(ctx context.Context) (BusinessMetricsStats, error) {
	if p == nil || p.queryer == nil {
		return BusinessMetricsStats{}, fmt.Errorf("business metrics queryer is nil")
	}
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	approvalTickets, err := queryBusinessApprovalTicketCounts(ctx, p.queryer, businessApprovalTicketsByStatusQuery)
	if err != nil {
		return BusinessMetricsStats{}, err
	}
	approvalPendingAges, err := queryBusinessApprovalPendingAges(ctx, p.queryer, businessApprovalPendingOldestAgeQuery)
	if err != nil {
		return BusinessMetricsStats{}, err
	}
	batchApprovalTickets, err := queryBusinessBatchApprovalTicketCounts(ctx, p.queryer, businessBatchApprovalTicketsByStatusQuery)
	if err != nil {
		return BusinessMetricsStats{}, err
	}
	batchApprovalPendingAges, err := queryBusinessBatchApprovalPendingAges(ctx, p.queryer, businessBatchApprovalPendingOldestAgeQuery)
	if err != nil {
		return BusinessMetricsStats{}, err
	}
	batchApprovalFailedChildren, err := queryBusinessBatchApprovalFailedChildCounts(ctx, p.queryer, businessBatchApprovalFailedChildrenQuery)
	if err != nil {
		return BusinessMetricsStats{}, err
	}
	recentWindowSeconds := int64(p.recentAuditWindow.Seconds())
	approvalAuditActions, err := queryBusinessApprovalAuditActionCounts(ctx, p.queryer, businessApprovalAuditActionsRecentQuery, recentWindowSeconds)
	if err != nil {
		return BusinessMetricsStats{}, err
	}
	approvalFailureAuditActions, err := queryBusinessApprovalAuditActionCounts(ctx, p.queryer, businessApprovalFailureAuditActionsRecentQuery, recentWindowSeconds)
	if err != nil {
		return BusinessMetricsStats{}, err
	}

	return NormalizeBusinessMetricsStats(BusinessMetricsStats{
		ApprovalTickets:             approvalTickets,
		ApprovalPendingAges:         approvalPendingAges,
		BatchApprovalTickets:        batchApprovalTickets,
		BatchApprovalPendingAges:    batchApprovalPendingAges,
		BatchApprovalFailedChildren: batchApprovalFailedChildren,
		ApprovalAuditActions:        approvalAuditActions,
		ApprovalFailureAuditActions: approvalFailureAuditActions,
	}), nil
}

// NormalizeBusinessMetricsStats caps business metric labels to the low-cardinality baseline.
func NormalizeBusinessMetricsStats(stats BusinessMetricsStats) BusinessMetricsStats {
	stats.ApprovalAuditActions = normalizeBusinessApprovalAuditActionCounts(stats.ApprovalAuditActions)
	stats.ApprovalFailureAuditActions = normalizeBusinessApprovalAuditActionCounts(stats.ApprovalFailureAuditActions)
	return stats
}

func normalizeBusinessApprovalAuditActionCounts(items []BusinessApprovalAuditActionCount) []BusinessApprovalAuditActionCount {
	if len(items) == 0 {
		return nil
	}

	totals := make(map[string]float64, len(items))
	for _, item := range items {
		action := normalizeBusinessApprovalAuditAction(item.Action)
		totals[action] += item.Count
	}

	actions := make([]string, 0, len(totals))
	for action := range totals {
		actions = append(actions, action)
	}
	sort.Strings(actions)

	result := make([]BusinessApprovalAuditActionCount, 0, len(actions))
	for _, action := range actions {
		result = append(result, BusinessApprovalAuditActionCount{
			Action: action,
			Count:  totals[action],
		})
	}
	return result
}

func normalizeBusinessApprovalAuditAction(action string) string {
	action = strings.TrimSpace(action)
	if _, ok := businessApprovalAuditActionAllowlist[action]; ok {
		return action
	}
	return BusinessApprovalAuditOtherAction
}

func queryBusinessApprovalTicketCounts(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]BusinessApprovalTicketCount, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query approval tickets by status: %w", err)
	}
	defer rows.Close()

	var counts []BusinessApprovalTicketCount
	for rows.Next() {
		var count BusinessApprovalTicketCount
		if err := rows.Scan(&count.Status, &count.OperationType, &count.Count); err != nil {
			return nil, fmt.Errorf("scan approval tickets by status: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read approval tickets by status: %w", err)
	}
	return counts, nil
}

func queryBusinessApprovalPendingAges(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]BusinessApprovalPendingAge, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query approval pending oldest age: %w", err)
	}
	defer rows.Close()

	var ages []BusinessApprovalPendingAge
	for rows.Next() {
		var age BusinessApprovalPendingAge
		if err := rows.Scan(&age.OperationType, &age.AgeSeconds); err != nil {
			return nil, fmt.Errorf("scan approval pending oldest age: %w", err)
		}
		ages = append(ages, age)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read approval pending oldest age: %w", err)
	}
	return ages, nil
}

func queryBusinessBatchApprovalTicketCounts(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]BusinessBatchApprovalTicketCount, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query batch approval tickets by status: %w", err)
	}
	defer rows.Close()

	var counts []BusinessBatchApprovalTicketCount
	for rows.Next() {
		var count BusinessBatchApprovalTicketCount
		if err := rows.Scan(&count.Status, &count.BatchType, &count.Count); err != nil {
			return nil, fmt.Errorf("scan batch approval tickets by status: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read batch approval tickets by status: %w", err)
	}
	return counts, nil
}

func queryBusinessBatchApprovalPendingAges(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]BusinessBatchApprovalPendingAge, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query batch approval pending oldest age: %w", err)
	}
	defer rows.Close()

	var ages []BusinessBatchApprovalPendingAge
	for rows.Next() {
		var age BusinessBatchApprovalPendingAge
		if err := rows.Scan(&age.BatchType, &age.AgeSeconds); err != nil {
			return nil, fmt.Errorf("scan batch approval pending oldest age: %w", err)
		}
		ages = append(ages, age)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read batch approval pending oldest age: %w", err)
	}
	return ages, nil
}

func queryBusinessBatchApprovalFailedChildCounts(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]BusinessBatchApprovalFailedChildCount, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query batch approval failed children: %w", err)
	}
	defer rows.Close()

	var counts []BusinessBatchApprovalFailedChildCount
	for rows.Next() {
		var count BusinessBatchApprovalFailedChildCount
		if err := rows.Scan(&count.BatchType, &count.Count); err != nil {
			return nil, fmt.Errorf("scan batch approval failed children: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read batch approval failed children: %w", err)
	}
	return counts, nil
}

func queryBusinessApprovalAuditActionCounts(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]BusinessApprovalAuditActionCount, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query approval audit actions: %w", err)
	}
	defer rows.Close()

	var counts []BusinessApprovalAuditActionCount
	for rows.Next() {
		var count BusinessApprovalAuditActionCount
		if err := rows.Scan(&count.Action, &count.Count); err != nil {
			return nil, fmt.Errorf("scan approval audit actions: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read approval audit actions: %w", err)
	}
	return counts, nil
}

type businessMetricsCollector struct {
	provider                              BusinessMetricsProvider
	approvalTicketsDesc                   *prometheus.Desc
	approvalPendingOldestAgeDesc          *prometheus.Desc
	batchApprovalTicketsDesc              *prometheus.Desc
	batchApprovalPendingOldestAgeDesc     *prometheus.Desc
	batchApprovalFailedChildrenDesc       *prometheus.Desc
	approvalAuditActionsRecentDesc        *prometheus.Desc
	approvalFailureAuditActionsRecentDesc *prometheus.Desc
	scrapeSuccessDesc                     *prometheus.Desc
}

// NewBusinessMetricsCollector creates a scrape-time collector for business observability aggregates.
func NewBusinessMetricsCollector(provider BusinessMetricsProvider) prometheus.Collector {
	return &businessMetricsCollector{
		provider: provider,
		approvalTicketsDesc: prometheus.NewDesc(
			"shepherd_business_approval_tickets",
			"Current approval tickets grouped by status and operation type.",
			[]string{"status", "operation_type"},
			nil,
		),
		approvalPendingOldestAgeDesc: prometheus.NewDesc(
			"shepherd_business_approval_pending_oldest_age_seconds",
			"Age in seconds of the oldest pending approval ticket grouped by operation type.",
			[]string{"operation_type"},
			nil,
		),
		batchApprovalTicketsDesc: prometheus.NewDesc(
			"shepherd_business_batch_approval_tickets",
			"Current batch approval tickets grouped by status and batch type.",
			[]string{"status", "batch_type"},
			nil,
		),
		batchApprovalPendingOldestAgeDesc: prometheus.NewDesc(
			"shepherd_business_batch_approval_pending_oldest_age_seconds",
			"Age in seconds of the oldest pending batch approval grouped by batch type.",
			[]string{"batch_type"},
			nil,
		),
		batchApprovalFailedChildrenDesc: prometheus.NewDesc(
			"shepherd_business_batch_approval_failed_children",
			"Current failed child count on batch approvals grouped by batch type.",
			[]string{"batch_type"},
			nil,
		),
		approvalAuditActionsRecentDesc: prometheus.NewDesc(
			"shepherd_business_approval_audit_actions_recent",
			"Approval-related audit actions observed within the recent fixed window, grouped by action.",
			[]string{"action"},
			nil,
		),
		approvalFailureAuditActionsRecentDesc: prometheus.NewDesc(
			"shepherd_business_approval_failure_audit_actions_recent",
			"Failure-related approval audit actions observed within the recent fixed window, grouped by action.",
			[]string{"action"},
			nil,
		),
		scrapeSuccessDesc: prometheus.NewDesc(
			"shepherd_business_metrics_scrape_success",
			"Whether business metrics collection succeeded for this scrape.",
			nil,
			nil,
		),
	}
}

// WithBusinessMetrics registers business observability metrics backed by pgx.
func WithBusinessMetrics(pool *pgxpool.Pool, timeout time.Duration) Option {
	return WithBusinessMetricsProvider(NewPGXBusinessMetricsProvider(pool, timeout))
}

// WithBusinessMetricsProvider registers business observability metrics with a custom provider.
func WithBusinessMetricsProvider(provider BusinessMetricsProvider) Option {
	return WithCollector(NewBusinessMetricsCollector(provider))
}

func (c *businessMetricsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.approvalTicketsDesc
	ch <- c.approvalPendingOldestAgeDesc
	ch <- c.batchApprovalTicketsDesc
	ch <- c.batchApprovalPendingOldestAgeDesc
	ch <- c.batchApprovalFailedChildrenDesc
	ch <- c.approvalAuditActionsRecentDesc
	ch <- c.approvalFailureAuditActionsRecentDesc
	ch <- c.scrapeSuccessDesc
}

func (c *businessMetricsCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil {
		return
	}
	if c.provider == nil {
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccessDesc, prometheus.GaugeValue, 0)
		return
	}

	ctx, cancel := collectorScrapeContext()
	defer cancel()

	stats, err := c.provider.BusinessMetrics(ctx)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccessDesc, prometheus.GaugeValue, 0)
		return
	}
	stats = NormalizeBusinessMetricsStats(stats)

	ch <- prometheus.MustNewConstMetric(c.scrapeSuccessDesc, prometheus.GaugeValue, 1)
	for _, count := range stats.ApprovalTickets {
		ch <- prometheus.MustNewConstMetric(c.approvalTicketsDesc, prometheus.GaugeValue, count.Count, count.Status, count.OperationType)
	}
	for _, age := range stats.ApprovalPendingAges {
		ch <- prometheus.MustNewConstMetric(c.approvalPendingOldestAgeDesc, prometheus.GaugeValue, age.AgeSeconds, age.OperationType)
	}
	for _, count := range stats.BatchApprovalTickets {
		ch <- prometheus.MustNewConstMetric(c.batchApprovalTicketsDesc, prometheus.GaugeValue, count.Count, count.Status, count.BatchType)
	}
	for _, age := range stats.BatchApprovalPendingAges {
		ch <- prometheus.MustNewConstMetric(c.batchApprovalPendingOldestAgeDesc, prometheus.GaugeValue, age.AgeSeconds, age.BatchType)
	}
	for _, count := range stats.BatchApprovalFailedChildren {
		ch <- prometheus.MustNewConstMetric(c.batchApprovalFailedChildrenDesc, prometheus.GaugeValue, count.Count, count.BatchType)
	}
	for _, count := range stats.ApprovalAuditActions {
		ch <- prometheus.MustNewConstMetric(c.approvalAuditActionsRecentDesc, prometheus.GaugeValue, count.Count, count.Action)
	}
	for _, count := range stats.ApprovalFailureAuditActions {
		ch <- prometheus.MustNewConstMetric(c.approvalFailureAuditActionsRecentDesc, prometheus.GaugeValue, count.Count, count.Action)
	}
}
