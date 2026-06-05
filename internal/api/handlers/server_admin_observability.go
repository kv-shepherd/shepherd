package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/observability"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	minAdminTraceLookbackMinutes = 5
	maxAdminTraceLookbackMinutes = 1440
	maxAdminTraceLimit           = 500
)

// GetAdminTraceSummary handles GET /admin/observability/traces.
func (s *Server) GetAdminTraceSummary(c *gin.Context, params generated.GetAdminTraceSummaryParams) {
	if !requireGlobalPermission(c, "observability:read") {
		return
	}
	if params.LookbackMinutes != 0 && (params.LookbackMinutes < minAdminTraceLookbackMinutes || params.LookbackMinutes > maxAdminTraceLookbackMinutes) {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "lookback_minutes must be between 5 and 1440",
		})
		return
	}
	if params.Limit != 0 && (params.Limit < 1 || params.Limit > maxAdminTraceLimit) {
		c.JSON(http.StatusBadRequest, generated.Error{
			Code:    "INVALID_REQUEST",
			Message: "limit must be between 1 and 500",
		})
		return
	}
	if s.traceSummaryProvider == nil {
		c.JSON(http.StatusServiceUnavailable, generated.Error{
			Code:    "TRACE_QUERY_UNAVAILABLE",
			Message: "trace query backend is not configured",
		})
		return
	}

	filter := observability.TraceSummaryFilter{
		Limit: params.Limit,
		Route: params.Route,
	}
	if params.LookbackMinutes > 0 {
		filter.Lookback = time.Duration(params.LookbackMinutes) * time.Minute
	}

	summary, err := s.traceSummaryProvider.TraceSummary(c.Request.Context(), filter)
	if err != nil {
		logger.Warn("failed to query trace summary", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, generated.Error{
			Code:    "TRACE_QUERY_UNAVAILABLE",
			Message: "trace query backend is unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, toAdminTraceSummaryResponse(summary))
}

// GetAdminAuditSignals handles GET /admin/observability/audit-signals.
func (s *Server) GetAdminAuditSignals(c *gin.Context) {
	if !requireGlobalPermission(c, "observability:read") {
		return
	}
	if s.businessMetrics == nil {
		c.JSON(http.StatusServiceUnavailable, generated.Error{
			Code:    "AUDIT_SIGNAL_UNAVAILABLE",
			Message: "audit signal backend is not configured",
		})
		return
	}

	stats, err := s.businessMetrics.BusinessMetrics(c.Request.Context())
	if err != nil {
		logger.Warn("failed to query audit business signals", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, generated.Error{
			Code:    "AUDIT_SIGNAL_UNAVAILABLE",
			Message: "audit signal backend is unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, toAdminAuditSignalResponse(time.Now().UTC(), stats))
}

func toAdminTraceSummaryResponse(summary observability.TraceSummary) generated.AdminObservabilityTraceSummary {
	endpoints := make([]generated.AdminObservabilityTraceEndpoint, 0, len(summary.Endpoints))
	for _, endpoint := range summary.Endpoints {
		endpoints = append(endpoints, generated.AdminObservabilityTraceEndpoint{
			Route:          endpoint.Route,
			RequestCount:   endpoint.RequestCount,
			ErrorCount:     endpoint.ErrorCount,
			ErrorRate:      endpoint.ErrorRate,
			P95Ms:          endpoint.P95Ms,
			AvgMs:          endpoint.AvgMs,
			MaxMs:          endpoint.MaxMs,
			SlowestTraceId: endpoint.SlowestTraceID,
		})
	}

	slowTraces := make([]generated.AdminObservabilityTraceSample, 0, len(summary.SlowTraces))
	for _, trace := range summary.SlowTraces {
		slowTraces = append(slowTraces, generated.AdminObservabilityTraceSample{
			TraceId:    trace.TraceID,
			RootName:   trace.RootName,
			Route:      trace.Route,
			DurationMs: trace.DurationMs,
			StatusCode: trace.StatusCode,
			Error:      trace.Error,
			StartedAt:  trace.StartedAt,
		})
	}

	dependencies := make([]generated.AdminObservabilitySpanGroup, 0, len(summary.Dependencies))
	for _, dependency := range summary.Dependencies {
		dependencies = append(dependencies, generated.AdminObservabilitySpanGroup{
			Category:   toAdminSpanGroupCategory(dependency.Category),
			Name:       dependency.Name,
			SpanCount:  dependency.SpanCount,
			ErrorCount: dependency.ErrorCount,
			P95Ms:      dependency.P95Ms,
			MaxMs:      dependency.MaxMs,
		})
	}

	return generated.AdminObservabilityTraceSummary{
		GeneratedAt:   summary.GeneratedAt,
		Source:        summary.Source,
		Status:        summary.Status,
		WindowSeconds: summary.WindowSeconds,
		Endpoints:     endpoints,
		SlowTraces:    slowTraces,
		Dependencies:  dependencies,
	}
}

func toAdminSpanGroupCategory(category string) generated.AdminObservabilitySpanGroupCategory {
	switch category {
	case "business":
		return generated.Business
	case "database":
		return generated.Database
	case "kubevirt":
		return generated.Kubevirt
	case "provider":
		return generated.Provider
	case "worker":
		return generated.Worker
	default:
		return generated.Internal
	}
}

func toAdminAuditSignalResponse(generatedAt time.Time, stats observability.BusinessMetricsStats) generated.AdminObservabilityAuditSignalSummary {
	return generated.AdminObservabilityAuditSignalSummary{
		GeneratedAt:                 generatedAt,
		Status:                      "ok",
		WindowSeconds:               int64(time.Hour.Seconds()),
		ApprovalTickets:             toApprovalTicketCounts(stats.ApprovalTickets),
		ApprovalPendingAges:         toApprovalPendingAges(stats.ApprovalPendingAges),
		BatchApprovalTickets:        toBatchApprovalTicketCounts(stats.BatchApprovalTickets),
		BatchApprovalPendingAges:    toBatchApprovalPendingAges(stats.BatchApprovalPendingAges),
		BatchApprovalFailedChildren: toBatchApprovalFailedChildCounts(stats.BatchApprovalFailedChildren),
		ApprovalAuditActions:        toAuditActionCounts(stats.ApprovalAuditActions),
		ApprovalFailureAuditActions: toAuditActionCounts(stats.ApprovalFailureAuditActions),
	}
}

func toApprovalTicketCounts(items []observability.BusinessApprovalTicketCount) []generated.AdminObservabilityApprovalTicketCount {
	result := make([]generated.AdminObservabilityApprovalTicketCount, 0, len(items))
	for _, item := range items {
		result = append(result, generated.AdminObservabilityApprovalTicketCount{
			Status:        item.Status,
			OperationType: item.OperationType,
			Count:         item.Count,
		})
	}
	return result
}

func toApprovalPendingAges(items []observability.BusinessApprovalPendingAge) []generated.AdminObservabilityApprovalPendingAge {
	result := make([]generated.AdminObservabilityApprovalPendingAge, 0, len(items))
	for _, item := range items {
		result = append(result, generated.AdminObservabilityApprovalPendingAge{
			OperationType: item.OperationType,
			AgeSeconds:    item.AgeSeconds,
		})
	}
	return result
}

func toBatchApprovalTicketCounts(items []observability.BusinessBatchApprovalTicketCount) []generated.AdminObservabilityBatchApprovalTicketCount {
	result := make([]generated.AdminObservabilityBatchApprovalTicketCount, 0, len(items))
	for _, item := range items {
		result = append(result, generated.AdminObservabilityBatchApprovalTicketCount{
			Status:    item.Status,
			BatchType: item.BatchType,
			Count:     item.Count,
		})
	}
	return result
}

func toBatchApprovalPendingAges(items []observability.BusinessBatchApprovalPendingAge) []generated.AdminObservabilityBatchApprovalPendingAge {
	result := make([]generated.AdminObservabilityBatchApprovalPendingAge, 0, len(items))
	for _, item := range items {
		result = append(result, generated.AdminObservabilityBatchApprovalPendingAge{
			BatchType:  item.BatchType,
			AgeSeconds: item.AgeSeconds,
		})
	}
	return result
}

func toBatchApprovalFailedChildCounts(items []observability.BusinessBatchApprovalFailedChildCount) []generated.AdminObservabilityBatchApprovalFailedChildCount {
	result := make([]generated.AdminObservabilityBatchApprovalFailedChildCount, 0, len(items))
	for _, item := range items {
		result = append(result, generated.AdminObservabilityBatchApprovalFailedChildCount{
			BatchType: item.BatchType,
			Count:     item.Count,
		})
	}
	return result
}

func toAuditActionCounts(items []observability.BusinessApprovalAuditActionCount) []generated.AdminObservabilityAuditActionCount {
	result := make([]generated.AdminObservabilityAuditActionCount, 0, len(items))
	for _, item := range items {
		result = append(result, generated.AdminObservabilityAuditActionCount{
			Action: item.Action,
			Count:  item.Count,
		})
	}
	return result
}
