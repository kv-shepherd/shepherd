package handlers

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestListAuditLogs_FiltersByApprovalDecisionAndPlacementReason(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	mustCreateAuditLog := func(action, resourceID string, details map[string]interface{}) {
		t.Helper()
		_, err := client.AuditLog.Create().
			SetID("audit-" + uuid.NewString()).
			SetAction(action).
			SetResourceType("ticket").
			SetResourceID(resourceID).
			SetActor("admin-1").
			SetDetails(details).
			Save(ctx)
		if err != nil {
			t.Fatalf("create audit log %s: %v", resourceID, err)
		}
	}

	mustCreateAuditLog("approval.validation_failed", "ticket-1", map[string]interface{}{
		"decision": "validation_failed",
		"placement_evaluation": map[string]interface{}{
			"reason_code":    "CLUSTER_POLICY_DENIED",
			"reason_message": "storage class is not allowed",
		},
	})
	mustCreateAuditLog("approval.validation_failed", "ticket-2", map[string]interface{}{
		"decision": "validation_failed",
		"placement_evaluation": map[string]interface{}{
			"reason_code":    "VALIDATION_FAILED",
			"reason_message": "cluster is missing GPU capability",
		},
	})
	mustCreateAuditLog("approval.approved", "ticket-3", map[string]interface{}{
		"decision": "approved",
		"placement_evaluation": map[string]interface{}{
			"eligible": true,
		},
	})

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs?approval_decision=validation_failed&placement_reason_code=CLUSTER_POLICY_DENIED",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{
		ApprovalDecision:    "validation_failed",
		PlacementReasonCode: "CLUSTER_POLICY_DENIED",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].ResourceId != "ticket-1" {
		t.Fatalf("resource_id = %q, want ticket-1", resp.Items[0].ResourceId)
	}
	if resp.Items[0].ApprovalDecision != "validation_failed" {
		t.Fatalf("approval_decision = %q, want validation_failed", resp.Items[0].ApprovalDecision)
	}
	if resp.Items[0].PlacementSummary == nil {
		t.Fatal("placement_summary = nil, want non-nil")
	}
	if resp.Items[0].PlacementSummary.ReasonCode != "CLUSTER_POLICY_DENIED" {
		t.Fatalf("placement_summary.reason_code = %q, want CLUSTER_POLICY_DENIED", resp.Items[0].PlacementSummary.ReasonCode)
	}
	if resp.Items[0].Details == nil {
		t.Fatal("details = nil, want non-nil")
	}
	rawPlacement, ok := resp.Items[0].Details["placement_evaluation"].(map[string]interface{})
	if !ok {
		t.Fatalf("placement_evaluation = %#v, want object", resp.Items[0].Details["placement_evaluation"])
	}
	if rawPlacement["reason_code"] != "CLUSTER_POLICY_DENIED" {
		t.Fatalf("reason_code = %v, want CLUSTER_POLICY_DENIED", rawPlacement["reason_code"])
	}
}

func TestListAuditLogs_FiltersByPlacementAdvisoryCode(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	mustCreateAuditLog := func(action, resourceID string, details map[string]interface{}) {
		t.Helper()
		_, err := client.AuditLog.Create().
			SetID("audit-" + uuid.NewString()).
			SetAction(action).
			SetResourceType("ticket").
			SetResourceID(resourceID).
			SetActor("admin-1").
			SetDetails(details).
			Save(ctx)
		if err != nil {
			t.Fatalf("create audit log %s: %v", resourceID, err)
		}
	}

	mustCreateAuditLog("approval.approved", "ticket-1", map[string]interface{}{
		"decision": "approved",
		"placement_evaluation": map[string]interface{}{
			"eligible":         true,
			"advisory_code":    "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
			"advisory_message": "clone may fall back to host-assisted copy",
		},
	})
	mustCreateAuditLog("approval.approved", "ticket-2", map[string]interface{}{
		"decision": "approved",
		"placement_evaluation": map[string]interface{}{
			"eligible": true,
		},
	})

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs?placement_advisory_code=PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{
		PlacementAdvisoryCode: "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].ResourceId != "ticket-1" {
		t.Fatalf("resource_id = %q, want ticket-1", resp.Items[0].ResourceId)
	}
	if resp.Items[0].PlacementSummary == nil {
		t.Fatal("placement_summary = nil, want non-nil")
	}
	if resp.Items[0].PlacementSummary.AdvisoryCode != "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY" {
		t.Fatalf(
			"placement_summary.advisory_code = %q, want PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
			resp.Items[0].PlacementSummary.AdvisoryCode,
		)
	}
	if resp.Items[0].Details == nil {
		t.Fatal("details = nil, want non-nil")
	}
	rawPlacement, ok := resp.Items[0].Details["placement_evaluation"].(map[string]interface{})
	if !ok {
		t.Fatalf("placement_evaluation = %#v, want object", resp.Items[0].Details["placement_evaluation"])
	}
	if rawPlacement["advisory_code"] != "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY" {
		t.Fatalf("advisory_code = %v, want PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY", rawPlacement["advisory_code"])
	}
}

func TestListAuditLogs_PlacementReasonFilterExcludesNonApprovalAuditEntries(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	_, err := client.AuditLog.Create().
		SetID("audit-" + uuid.NewString()).
		SetAction("vm.create").
		SetResourceType("vm").
		SetResourceID("vm-1").
		SetActor("user-a").
		SetDetails(map[string]interface{}{
			"placement_evaluation": map[string]interface{}{
				"reason_code": "CLUSTER_POLICY_DENIED",
			},
		}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create vm audit log: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs?placement_reason_code=CLUSTER_POLICY_DENIED",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{
		PlacementReasonCode: "CLUSTER_POLICY_DENIED",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 0 {
		t.Fatalf("items len = %d, want 0", got)
	}
}
