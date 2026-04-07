package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entdirectorysyncjob "kv-shepherd.io/shepherd/ent/directorysyncjob"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
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

func TestListAuditLogs_SupportsQuickSearchAcrossActionActorAndResource(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	mustCreateAuditLog := func(action, actor, resourceType, resourceID string) {
		t.Helper()
		_, err := client.AuditLog.Create().
			SetID("audit-" + uuid.NewString()).
			SetAction(action).
			SetActor(actor).
			SetResourceType(resourceType).
			SetResourceID(resourceID).
			SetDetails(map[string]interface{}{}).
			Save(ctx)
		if err != nil {
			t.Fatalf("create audit log %s: %v", resourceID, err)
		}
	}

	mustCreateAuditLog("vm.create", "alice", "vm", "vm-frontend-1")
	mustCreateAuditLog("service.update", "bob", "service", "svc-payments")
	mustCreateAuditLog("user.password_change", "carol", "user", "user-carol")

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs?search=payments",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{
		Search: "payments",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].ResourceId != "svc-payments" {
		t.Fatalf("resource_id = %q, want svc-payments", resp.Items[0].ResourceId)
	}
}

func TestListAuditLogs_FiltersByCategory(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	createLog := func(action, actor, resourceType, resourceID string) {
		t.Helper()
		_, err := client.AuditLog.Create().
			SetID("audit-" + uuid.NewString()).
			SetAction(action).
			SetActor(actor).
			SetResourceType(resourceType).
			SetResourceID(resourceID).
			SetDetails(map[string]interface{}{}).
			Save(ctx)
		if err != nil {
			t.Fatalf("create audit log %s: %v", resourceID, err)
		}
	}

	createLog("vm.request", "user-1", "ticket", "ticket-1")
	createLog("approval.approved", "approver-1", "ticket", "ticket-2")
	createLog("service.update", "admin-1", "service", "svc-1")
	createLog("auth_provider.directory_sync", "system:directory-enrichment-scheduler", "directory_sync_job", "job-1")

	tests := []struct {
		name     string
		category string
		wantID   string
	}{
		{name: "requests", category: "requests", wantID: "ticket-1"},
		{name: "approvals", category: "approvals", wantID: "ticket-2"},
		{name: "resource changes", category: "resource_changes", wantID: "svc-1"},
		{name: "system tasks", category: "system_tasks", wantID: "job-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := newAuthedGinContext(
				t,
				http.MethodGet,
				"/audit-logs?category="+tt.category,
				"",
				"admin-1",
				[]string{"audit:read", "platform:admin"},
			)
			srv.ListAuditLogs(c, generated.ListAuditLogsParams{Category: generated.ListAuditLogsParamsCategory(tt.category)})

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
			}

			var resp generated.AuditLogList
			mustDecodeJSON(t, w.Body.Bytes(), &resp)
			if got := len(resp.Items); got != 1 {
				t.Fatalf("items len = %d, want 1", got)
			}
			if resp.Items[0].ResourceId != tt.wantID {
				t.Fatalf("resource_id = %q, want %q", resp.Items[0].ResourceId, tt.wantID)
			}
		})
	}
}

func TestListAuditLogs_EnrichesReadableActorResourceAndTicketSummary(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	actor, err := client.User.Create().
		SetID("user-alice").
		SetUsername("alice").
		SetDisplayName("Alice Chen").
		SetEmail("alice@example.com").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	system, err := client.System.Create().
		SetID("system-payments").
		SetName("Payments").
		SetDescription("Handles payment flows").
		SetCreatedBy(actor.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}

	service, err := client.Service.Create().
		SetID("service-checkout").
		SetName("checkout-api").
		SetDescription("Checkout flow").
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	mustCreateApprovalTemplate(t, client, "template-ubuntu")
	mustCreateApprovalInstanceSize(t, client, "size-m4")

	approver, err := client.User.Create().
		SetID("user-bob").
		SetUsername("bob.ops").
		SetDisplayName("Bob Ops").
		SetEmail("bob@example.com").
		Save(ctx)
	if err != nil {
		t.Fatalf("create approver: %v", err)
	}

	eventID := "event-" + uuid.NewString()
	ticketID := "ticket-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(
		t,
		client,
		eventID,
		"ticket",
		ticketID,
		mustApprovalJSON(t, map[string]interface{}{
			"service_id":       service.ID,
			"template_id":      "template-ubuntu",
			"namespace":        "team-test",
			"requester_id":     actor.ID,
			"instance_size_id": "size-m4",
		}),
	)
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, actor.ID)
	if _, updateErr := client.Ticket.UpdateOneID(ticketID).SetApprover(approver.ID).Save(ctx); updateErr != nil {
		t.Fatalf("set approver: %v", updateErr)
	}

	_, err = client.AuditLog.Create().
		SetID("audit-" + uuid.NewString()).
		SetAction("service.update").
		SetResourceType("service").
		SetResourceID(service.ID).
		SetActor(actor.ID).
		SetDetails(map[string]interface{}{}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create service audit log: %v", err)
	}

	_, err = client.AuditLog.Create().
		SetID("audit-" + uuid.NewString()).
		SetAction("approval.approved").
		SetResourceType("ticket").
		SetResourceID(ticketID).
		SetActor(actor.ID).
		SetDetails(map[string]interface{}{
			"decision": "approved",
			"placement_evaluation": map[string]interface{}{
				"eligible":              true,
				"selected_cluster_name": "kubevirt-test02",
			},
		}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create ticket audit log: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 2 {
		t.Fatalf("items len = %d, want 2", got)
	}

	var serviceAudit *generated.AuditLog
	var ticketAudit *generated.AuditLog
	for i := range resp.Items {
		item := &resp.Items[i]
		switch item.ResourceType {
		case "service":
			serviceAudit = item
		case "ticket":
			ticketAudit = item
		}
	}

	if serviceAudit == nil {
		t.Fatal("service audit = nil, want non-nil")
	}
	if serviceAudit.ActorSummary == nil || serviceAudit.ActorSummary.DisplayName != "Alice Chen" {
		t.Fatalf("service actor_summary = %#v, want display name Alice Chen", serviceAudit.ActorSummary)
	}
	if serviceAudit.ResourceSummary == nil || serviceAudit.ResourceSummary.DisplayName != "checkout-api" {
		t.Fatalf("service resource_summary = %#v, want display name checkout-api", serviceAudit.ResourceSummary)
	}
	if serviceAudit.ResourceSummary.Secondary != "Payments" {
		t.Fatalf("service resource_summary.secondary = %q, want Payments", serviceAudit.ResourceSummary.Secondary)
	}

	if ticketAudit == nil {
		t.Fatal("ticket audit = nil, want non-nil")
	}
	if ticketAudit.TicketSummary == nil {
		t.Fatal("ticket_summary = nil, want non-nil")
	}
	if ticketAudit.TicketSummary.SystemName != "Payments" {
		t.Fatalf("ticket_summary.system_name = %q, want Payments", ticketAudit.TicketSummary.SystemName)
	}
	if ticketAudit.TicketSummary.ServiceName != "checkout-api" {
		t.Fatalf("ticket_summary.service_name = %q, want checkout-api", ticketAudit.TicketSummary.ServiceName)
	}
	if ticketAudit.TicketSummary.TemplateName != "Ubuntu 22.04" {
		t.Fatalf("ticket_summary.template_name = %q, want Ubuntu 22.04", ticketAudit.TicketSummary.TemplateName)
	}
	if ticketAudit.TicketSummary.RequesterDisplayName != "Alice Chen" {
		t.Fatalf("ticket_summary.requester_display_name = %q, want Alice Chen", ticketAudit.TicketSummary.RequesterDisplayName)
	}
	if ticketAudit.TicketSummary.RequesterUsername != "alice" {
		t.Fatalf("ticket_summary.requester_username = %q, want alice", ticketAudit.TicketSummary.RequesterUsername)
	}
	if ticketAudit.TicketSummary.ApproverDisplayName != "Bob Ops" {
		t.Fatalf("ticket_summary.approver_display_name = %q, want Bob Ops", ticketAudit.TicketSummary.ApproverDisplayName)
	}
	if ticketAudit.TicketSummary.ApproverUsername != "bob.ops" {
		t.Fatalf("ticket_summary.approver_username = %q, want bob.ops", ticketAudit.TicketSummary.ApproverUsername)
	}
	if ticketAudit.ResourceSummary == nil || ticketAudit.ResourceSummary.DisplayName != "checkout-api" {
		t.Fatalf("ticket resource_summary = %#v, want display name checkout-api", ticketAudit.ResourceSummary)
	}
	if ticketAudit.ResourceSummary.Tertiary != "Ubuntu 22.04 · M4 Large · 4 vCPU · 8 Gi · 80 Gi" {
		t.Fatalf("ticket resource_summary.tertiary = %q, want summary string", ticketAudit.ResourceSummary.Tertiary)
	}
}

func TestListAuditLogs_EnrichesBatchTicketItemsWithReadableScopeOwnerAndConfig(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	requester, err := client.User.Create().
		SetID("user-requester").
		SetUsername("alexchen").
		SetDisplayName("Alex Chen").
		SetEmail("alexchen@example.com").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create requester: %v", err)
	}

	approver, err := client.User.Create().
		SetID("user-approver").
		SetUsername("admin").
		SetDisplayName("Default Administrator").
		SetEmail("admin@example.com").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create approver: %v", err)
	}

	system, err := client.System.Create().
		SetID("system-shop").
		SetName("shop").
		SetDescription("Retail system").
		SetCreatedBy(requester.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}

	service, err := client.Service.Create().
		SetID("service-redis").
		SetName("redis").
		SetDescription("Redis cache").
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	mustCreateApprovalTemplate(t, client, "template-openeuler")
	mustCreateApprovalInstanceSize(t, client, "size-vnc")

	eventID := "event-" + uuid.NewString()
	ticketID := "ticket-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(
		t,
		client,
		eventID,
		"ticket",
		ticketID,
		mustApprovalJSON(t, map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"service_id":       service.ID,
					"template_id":      "template-openeuler",
					"instance_size_id": "size-vnc",
					"namespace":        "gtest1",
				},
				{
					"service_id":       service.ID,
					"template_id":      "template-openeuler",
					"instance_size_id": "size-vnc",
					"namespace":        "gtest1",
				},
			},
			"batch_item_count": 2,
		}),
	)
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, requester.ID)
	if _, updateErr := client.Ticket.UpdateOneID(ticketID).SetApprover(approver.ID).Save(ctx); updateErr != nil {
		t.Fatalf("set approver: %v", updateErr)
	}

	_, err = client.AuditLog.Create().
		SetID("audit-" + uuid.NewString()).
		SetAction("approval.batch_approved").
		SetResourceType("ticket").
		SetResourceID(ticketID).
		SetActor(approver.ID).
		SetDetails(map[string]interface{}{
			"decision": "batch_approved",
		}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	ticketAudit := resp.Items[0]
	if ticketAudit.TicketSummary == nil {
		t.Fatal("ticket_summary = nil, want non-nil")
	}
	if ticketAudit.TicketSummary.SystemName != "shop" {
		t.Fatalf("ticket_summary.system_name = %q, want shop", ticketAudit.TicketSummary.SystemName)
	}
	if ticketAudit.TicketSummary.ServiceName != "redis" {
		t.Fatalf("ticket_summary.service_name = %q, want redis", ticketAudit.TicketSummary.ServiceName)
	}
	if ticketAudit.TicketSummary.OwnerDisplayName != "Alex Chen" {
		t.Fatalf("ticket_summary.owner_display_name = %q, want Alex Chen", ticketAudit.TicketSummary.OwnerDisplayName)
	}
	if ticketAudit.TicketSummary.OwnerUsername != "alexchen" {
		t.Fatalf("ticket_summary.owner_username = %q, want alexchen", ticketAudit.TicketSummary.OwnerUsername)
	}
	if ticketAudit.TicketSummary.BatchCount != 2 {
		t.Fatalf("ticket_summary.batch_count = %d, want 2", ticketAudit.TicketSummary.BatchCount)
	}
	if len(ticketAudit.TicketSummary.Items) != 2 {
		t.Fatalf("ticket_summary.items len = %d, want 2", len(ticketAudit.TicketSummary.Items))
	}
	firstItem := ticketAudit.TicketSummary.Items[0]
	if firstItem.SystemName != "shop" {
		t.Fatalf("item.system_name = %q, want shop", firstItem.SystemName)
	}
	if firstItem.ServiceName != "redis" {
		t.Fatalf("item.service_name = %q, want redis", firstItem.ServiceName)
	}
	if firstItem.Namespace != "gtest1" {
		t.Fatalf("item.namespace = %q, want gtest1", firstItem.Namespace)
	}
	if firstItem.TemplateName != "Ubuntu 22.04" {
		t.Fatalf("item.template_name = %q, want Ubuntu 22.04", firstItem.TemplateName)
	}
	if firstItem.InstanceSizeName != "M4 Large" {
		t.Fatalf("item.instance_size_name = %q, want M4 Large", firstItem.InstanceSizeName)
	}
	if firstItem.TargetCpuCores != 4 || firstItem.TargetMemoryGi != 8 || firstItem.TargetDiskGb != 80 {
		t.Fatalf("item resources = cpu=%v memory=%v disk=%d, want 4/8/80", firstItem.TargetCpuCores, firstItem.TargetMemoryGi, firstItem.TargetDiskGb)
	}
	if firstItem.OwnerDisplayName != "Alex Chen" {
		t.Fatalf("item.owner_display_name = %q, want Alex Chen", firstItem.OwnerDisplayName)
	}
}

func TestListAuditLogs_BackfillsBatchTicketItemsFromChildTickets(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	requester := mustCreateUser(t, client, "requester-batch", "alexchen")
	if _, err := client.User.UpdateOneID(requester.ID).
		SetDisplayName("Alex Chen").
		SetEmail("alexchen@example.com").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("update requester: %v", err)
	}
	approver := mustCreateUser(t, client, "approver-batch", "admin")
	if _, err := client.User.UpdateOneID(approver.ID).
		SetDisplayName("Default Administrator").
		SetEmail("admin@localhost").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("update approver: %v", err)
	}

	system, err := client.System.Create().
		SetID("system-shop").
		SetName("shop").
		SetCreatedBy(requester.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}

	service, err := client.Service.Create().
		SetID("service-redis").
		SetName("redis").
		SetDescription("Redis cache").
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	parentEventID := "event-parent-" + uuid.NewString()
	parentTicketID := "ticket-parent-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(
		t,
		client,
		parentEventID,
		"ticket",
		parentTicketID,
		mustApprovalJSON(t, map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"vm_id":             "vm-a",
					"request_vm_status": "STOPPED",
				},
				{
					"vm_id":             "vm-b",
					"request_vm_status": "STOPPED",
				},
			},
			"batch_item_count": 2,
		}),
	)
	mustCreateTicket(t, client, parentTicketID, parentEventID, entticket.OperationTypeDELETE, requester.ID)
	if _, updateErr := client.Ticket.UpdateOneID(parentTicketID).SetApprover(approver.ID).Save(ctx); updateErr != nil {
		t.Fatalf("set approver: %v", updateErr)
	}

	childPayloads := []domain.VMDeletePayload{
		{
			VMID:               "vm-a",
			VMName:             "redis-01",
			ClusterID:          "cluster-1",
			ClusterName:        "kubevirt-test02",
			ClusterEnvironment: "test",
			Namespace:          "gtest1",
			SystemID:           system.ID,
			SystemName:         system.Name,
			ServiceID:          service.ID,
			ServiceName:        service.Name,
			OwnerID:            requester.ID,
			OwnerDisplayName:   "Alex Chen",
			OwnerUsername:      "alexchen",
			TemplateID:         "template-openeuler",
			TemplateName:       "OpenEuler 22.03",
			InstanceSizeID:     "size-vnc",
			InstanceSizeName:   "M4 Large",
			RequestVMStatus:    "STOPPED",
			CurrentCPUCores:    4,
			CurrentMemoryGi:    8,
			CurrentDiskGB:      60,
		},
		{
			VMID:               "vm-b",
			VMName:             "redis-02",
			ClusterID:          "cluster-1",
			ClusterName:        "kubevirt-test02",
			ClusterEnvironment: "test",
			Namespace:          "gtest1",
			SystemID:           system.ID,
			SystemName:         system.Name,
			ServiceID:          service.ID,
			ServiceName:        service.Name,
			OwnerID:            requester.ID,
			OwnerDisplayName:   "Alex Chen",
			OwnerUsername:      "alexchen",
			TemplateID:         "template-openeuler",
			TemplateName:       "OpenEuler 22.03",
			InstanceSizeID:     "size-vnc",
			InstanceSizeName:   "M4 Large",
			RequestVMStatus:    "STOPPED",
			CurrentCPUCores:    4,
			CurrentMemoryGi:    8,
			CurrentDiskGB:      60,
		},
	}
	for idx, childPayload := range childPayloads {
		childEventID := fmt.Sprintf("event-child-%d-%s", idx+1, uuid.NewString())
		rawPayload, marshalErr := json.Marshal(childPayload)
		if marshalErr != nil {
			t.Fatalf("marshal child payload: %v", marshalErr)
		}
		mustCreateDomainEventWithAggregate(
			t,
			client,
			childEventID,
			"vm",
			childPayload.VMID,
			rawPayload,
		)
		if _, createErr := client.Ticket.Create().
			SetID(fmt.Sprintf("ticket-child-%d-%s", idx+1, uuid.NewString())).
			SetEventID(childEventID).
			SetOperationType(entticket.OperationTypeDELETE).
			SetStatus(entticket.StatusSUCCESS).
			SetRequester(requester.ID).
			SetApprover(approver.ID).
			SetParentTicketID(parentTicketID).
			Save(ctx); createErr != nil {
			t.Fatalf("create child ticket: %v", createErr)
		}
	}

	_, err = client.AuditLog.Create().
		SetID("audit-" + uuid.NewString()).
		SetAction("approval.batch_approved").
		SetResourceType("ticket").
		SetResourceID(parentTicketID).
		SetActor(approver.ID).
		SetDetails(map[string]interface{}{
			"decision": "batch_approved",
		}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}

	item := resp.Items[0]
	if item.TicketSummary == nil {
		t.Fatal("ticket_summary = nil, want non-nil")
	}
	if item.TicketSummary.SystemName != "shop" {
		t.Fatalf("ticket_summary.system_name = %q, want shop", item.TicketSummary.SystemName)
	}
	if item.TicketSummary.ServiceName != "redis" {
		t.Fatalf("ticket_summary.service_name = %q, want redis", item.TicketSummary.ServiceName)
	}
	if item.TicketSummary.OwnerDisplayName != "Alex Chen" {
		t.Fatalf("ticket_summary.owner_display_name = %q, want Alex Chen", item.TicketSummary.OwnerDisplayName)
	}
	if len(item.TicketSummary.Items) != 2 {
		t.Fatalf("ticket_summary.items len = %d, want 2", len(item.TicketSummary.Items))
	}
	if item.TicketSummary.Items[0].VmName != "redis-01" || item.TicketSummary.Items[1].VmName != "redis-02" {
		t.Fatalf("ticket_summary item vm names = %#v, want redis-01/redis-02", item.TicketSummary.Items)
	}
	if item.TicketSummary.Items[0].TemplateName != "OpenEuler 22.03" {
		t.Fatalf("ticket_summary.items[0].template_name = %q, want OpenEuler 22.03", item.TicketSummary.Items[0].TemplateName)
	}
	if item.TicketSummary.Items[0].CurrentCpuCores != 4 || item.TicketSummary.Items[0].CurrentMemoryGi != 8 || item.TicketSummary.Items[0].CurrentDiskGb != 60 {
		t.Fatalf("ticket_summary.items[0] resources = cpu=%v memory=%v disk=%d, want 4/8/60", item.TicketSummary.Items[0].CurrentCpuCores, item.TicketSummary.Items[0].CurrentMemoryGi, item.TicketSummary.Items[0].CurrentDiskGb)
	}
}

func TestListAuditLogs_BackfillsBatchCreateSummaryFromChildPayloadIDs(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	requester := mustCreateUser(t, client, "requester-batch-create", "alexchen")
	if _, err := client.User.UpdateOneID(requester.ID).
		SetDisplayName("Alex Chen").
		SetEmail("alexchen@example.com").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("update requester: %v", err)
	}
	approver := mustCreateUser(t, client, "approver-batch-create", "admin")
	if _, err := client.User.UpdateOneID(approver.ID).
		SetDisplayName("Default Administrator").
		SetEmail("admin@example.com").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("update approver: %v", err)
	}

	system, err := client.System.Create().
		SetID("system-billing").
		SetName("billing").
		SetCreatedBy(requester.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	service, err := client.Service.Create().
		SetID("service-api").
		SetName("api").
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	mustCreateApprovalTemplate(t, client, "template-batch-create")
	mustCreateApprovalInstanceSize(t, client, "size-batch-create")

	parentEventID := "event-parent-" + uuid.NewString()
	parentTicketID := "ticket-parent-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(
		t,
		client,
		parentEventID,
		"ticket",
		parentTicketID,
		mustApprovalJSON(t, map[string]interface{}{
			"items": []map[string]interface{}{
				{},
				{},
			},
			"batch_item_count": 2,
		}),
	)
	mustCreateTicket(t, client, parentTicketID, parentEventID, entticket.OperationTypeCREATE, requester.ID)
	if _, err := client.Ticket.UpdateOneID(parentTicketID).
		SetApprover(approver.ID).
		SetPlacementEvaluation(map[string]interface{}{
			"selected_cluster_name":        "KubeVirt Test 02",
			"selected_cluster_environment": "test",
		}).
		Save(ctx); err != nil {
		t.Fatalf("update parent ticket: %v", err)
	}

	for idx := range 2 {
		childEventID := fmt.Sprintf("event-child-create-%d-%s", idx+1, uuid.NewString())
		mustCreateDomainEventWithAggregate(
			t,
			client,
			childEventID,
			"vm",
			fmt.Sprintf("vm-create-%d", idx+1),
			mustApprovalJSON(t, map[string]interface{}{
				"service_id":       service.ID,
				"template_id":      "template-batch-create",
				"instance_size_id": "size-batch-create",
				"namespace":        "team-test",
				"owner_id":         requester.ID,
			}),
		)
		if _, err := client.Ticket.Create().
			SetID(fmt.Sprintf("ticket-child-create-%d-%s", idx+1, uuid.NewString())).
			SetEventID(childEventID).
			SetOperationType(entticket.OperationTypeCREATE).
			SetStatus(entticket.StatusSUCCESS).
			SetRequester(requester.ID).
			SetApprover(approver.ID).
			SetParentTicketID(parentTicketID).
			Save(ctx); err != nil {
			t.Fatalf("create child ticket: %v", err)
		}
	}

	if _, err := client.AuditLog.Create().
		SetID("audit-" + uuid.NewString()).
		SetAction("approval.batch_approved").
		SetResourceType("ticket").
		SetResourceID(parentTicketID).
		SetActor(approver.ID).
		SetDetails(map[string]interface{}{"decision": "batch_approved"}).
		Save(ctx); err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}

	item := resp.Items[0]
	if item.TicketSummary == nil {
		t.Fatal("ticket_summary = nil, want non-nil")
	}
	if item.TicketSummary.SystemName != "billing" {
		t.Fatalf("ticket_summary.system_name = %q, want billing", item.TicketSummary.SystemName)
	}
	if item.TicketSummary.ServiceName != "api" {
		t.Fatalf("ticket_summary.service_name = %q, want api", item.TicketSummary.ServiceName)
	}
	if item.TicketSummary.OwnerDisplayName != "Alex Chen" {
		t.Fatalf("ticket_summary.owner_display_name = %q, want Alex Chen", item.TicketSummary.OwnerDisplayName)
	}
	if item.TicketSummary.TemplateName != "Ubuntu 22.04" {
		t.Fatalf("ticket_summary.template_name = %q, want Ubuntu 22.04", item.TicketSummary.TemplateName)
	}
	if item.TicketSummary.InstanceSizeName != "M4 Large" {
		t.Fatalf("ticket_summary.instance_size_name = %q, want M4 Large", item.TicketSummary.InstanceSizeName)
	}
	if item.TicketSummary.Namespace != "team-test" {
		t.Fatalf("ticket_summary.namespace = %q, want team-test", item.TicketSummary.Namespace)
	}
	if item.TicketSummary.ClusterName != "KubeVirt Test 02" {
		t.Fatalf("ticket_summary.cluster_name = %q, want KubeVirt Test 02", item.TicketSummary.ClusterName)
	}
	if item.TicketSummary.ClusterEnvironment != "test" {
		t.Fatalf("ticket_summary.cluster_environment = %q, want test", item.TicketSummary.ClusterEnvironment)
	}
	if len(item.TicketSummary.Items) != 2 {
		t.Fatalf("ticket_summary.items len = %d, want 2", len(item.TicketSummary.Items))
	}
	if item.TicketSummary.Items[0].SystemName != "billing" || item.TicketSummary.Items[0].ServiceName != "api" {
		t.Fatalf("ticket_summary.items[0] scope = %q / %q, want billing / api", item.TicketSummary.Items[0].SystemName, item.TicketSummary.Items[0].ServiceName)
	}
	if item.TicketSummary.Items[0].TemplateName != "Ubuntu 22.04" || item.TicketSummary.Items[0].InstanceSizeName != "M4 Large" {
		t.Fatalf("ticket_summary.items[0] catalog = %q / %q, want Ubuntu 22.04 / M4 Large", item.TicketSummary.Items[0].TemplateName, item.TicketSummary.Items[0].InstanceSizeName)
	}
}

func TestListAuditLogs_InfersHistoricalBatchVMContextFromSparseEvents(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	requester := mustCreateUser(t, client, "requester-history", "alexchen")
	if _, err := client.User.UpdateOneID(requester.ID).
		SetDisplayName("Alex Chen").
		SetEmail("alexchen@example.com").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("update requester: %v", err)
	}
	approver := mustCreateUser(t, client, "approver-history", "admin")
	if _, err := client.User.UpdateOneID(approver.ID).
		SetDisplayName("Default Administrator").
		SetEmail("admin@example.com").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("update approver: %v", err)
	}

	system, err := client.System.Create().
		SetID("system-shop").
		SetName("shop").
		SetCreatedBy(requester.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	if _, err := client.Service.Create().
		SetID("service-redis-api").
		SetName("redis-api").
		SetDescription("Redis API").
		SetSystem(system).
		Save(ctx); err != nil {
		t.Fatalf("create service: %v", err)
	}
	mustCreateClusterWithLabels(t, client, "cluster-test-1", "kubevirt-test02", "KubeVirt Test 02", entcluster.EnvironmentTest)

	parentEventID := "event-parent-history-" + uuid.NewString()
	parentTicketID := "ticket-parent-history-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(
		t,
		client,
		parentEventID,
		"ticket",
		parentTicketID,
		mustApprovalJSON(t, map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"vm_id":             "vm-hist-1",
					"request_vm_status": "STOPPED",
				},
				{
					"vm_id":             "vm-hist-2",
					"request_vm_status": "STOPPED",
				},
			},
			"batch_item_count": 2,
		}),
	)
	mustCreateTicket(t, client, parentTicketID, parentEventID, entticket.OperationTypeDELETE, requester.ID)
	if _, err := client.Ticket.UpdateOneID(parentTicketID).SetApprover(approver.ID).Save(ctx); err != nil {
		t.Fatalf("set approver: %v", err)
	}

	historicalPayloads := []struct {
		vmID   string
		vmName string
	}{
		{vmID: "vm-hist-1", vmName: "gtest1-shop-redis-api-01"},
		{vmID: "vm-hist-2", vmName: "gtest1-shop-redis-api-02"},
	}
	for idx, item := range historicalPayloads {
		modifyEventID := fmt.Sprintf("event-modify-history-%d-%s", idx+1, uuid.NewString())
		mustCreateDomainEventWithAggregate(
			t,
			client,
			modifyEventID,
			"vm",
			item.vmID,
			mustApprovalJSON(t, map[string]interface{}{
				"vm_id":             item.vmID,
				"vm_name":           item.vmName,
				"cluster_id":        "cluster-test-1",
				"namespace":         "gtest1",
				"request_vm_status": "RUNNING",
				"current_cpu_cores": 4,
				"current_memory_gi": 8,
				"current_disk_gb":   60,
				"operation":         "stop",
				"actor":             "user-default-admin",
			}),
		)

		childEventID := fmt.Sprintf("event-child-history-%d-%s", idx+1, uuid.NewString())
		mustCreateDomainEventWithAggregate(
			t,
			client,
			childEventID,
			"vm",
			item.vmID,
			mustApprovalJSON(t, map[string]interface{}{
				"vm_id":             item.vmID,
				"vm_name":           item.vmName,
				"cluster_id":        "cluster-test-1",
				"namespace":         "gtest1",
				"request_vm_status": "STOPPED",
			}),
		)
		if _, err := client.Ticket.Create().
			SetID(fmt.Sprintf("ticket-child-history-%d-%s", idx+1, uuid.NewString())).
			SetEventID(childEventID).
			SetOperationType(entticket.OperationTypeDELETE).
			SetStatus(entticket.StatusSUCCESS).
			SetRequester(requester.ID).
			SetApprover(approver.ID).
			SetParentTicketID(parentTicketID).
			Save(ctx); err != nil {
			t.Fatalf("create child ticket: %v", err)
		}
	}

	if _, err := client.AuditLog.Create().
		SetID("audit-" + uuid.NewString()).
		SetAction("approval.batch_approved").
		SetResourceType("ticket").
		SetResourceID(parentTicketID).
		SetActor(approver.ID).
		SetDetails(map[string]interface{}{"decision": "batch_approved"}).
		Save(ctx); err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}

	item := resp.Items[0]
	if item.TicketSummary == nil {
		t.Fatal("ticket_summary = nil, want non-nil")
	}
	if item.TicketSummary.SystemName != "shop" {
		t.Fatalf("ticket_summary.system_name = %q, want shop", item.TicketSummary.SystemName)
	}
	if item.TicketSummary.ServiceName != "redis-api" {
		t.Fatalf("ticket_summary.service_name = %q, want redis-api", item.TicketSummary.ServiceName)
	}
	if item.TicketSummary.OwnerDisplayName != "Alex Chen" {
		t.Fatalf("ticket_summary.owner_display_name = %q, want Alex Chen", item.TicketSummary.OwnerDisplayName)
	}
	if item.TicketSummary.ClusterName != "KubeVirt Test 02" {
		t.Fatalf("ticket_summary.cluster_name = %q, want KubeVirt Test 02", item.TicketSummary.ClusterName)
	}
	if item.TicketSummary.ClusterEnvironment != "test" {
		t.Fatalf("ticket_summary.cluster_environment = %q, want test", item.TicketSummary.ClusterEnvironment)
	}
	if len(item.TicketSummary.Items) != 2 {
		t.Fatalf("ticket_summary.items len = %d, want 2", len(item.TicketSummary.Items))
	}
	if item.TicketSummary.Items[0].VmName != "gtest1-shop-redis-api-01" {
		t.Fatalf("ticket_summary.items[0].vm_name = %q, want gtest1-shop-redis-api-01", item.TicketSummary.Items[0].VmName)
	}
	if item.TicketSummary.Items[0].CurrentCpuCores != 4 || item.TicketSummary.Items[0].CurrentMemoryGi != 8 || item.TicketSummary.Items[0].CurrentDiskGb != 60 {
		t.Fatalf("ticket_summary.items[0] resources = cpu=%v memory=%v disk=%d, want 4/8/60", item.TicketSummary.Items[0].CurrentCpuCores, item.TicketSummary.Items[0].CurrentMemoryGi, item.TicketSummary.Items[0].CurrentDiskGb)
	}
}

func TestListAuditLogs_EnrichesDirectorySyncJobSummary(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	provider, err := client.AuthProvider.Create().
		SetID("provider-sync").
		SetName("corp-directory").
		SetAuthType("ldap").
		SetCreatedBy("admin-1").
		SetConfig(map[string]interface{}{}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	job, err := client.DirectorySyncJob.Create().
		SetID("job-sync-1").
		SetAuthProviderID(provider.ID).
		SetRequestSnapshot(map[string]interface{}{}).
		SetSyncMode(entdirectorysyncjob.SyncModeScheduledEnrichment).
		SetStatus(entdirectorysyncjob.StatusCompleted).
		SetTriggeredBy("system:directory-enrichment-scheduler").
		Save(ctx)
	if err != nil {
		t.Fatalf("create directory sync job: %v", err)
	}

	_, err = client.AuditLog.Create().
		SetID("audit-" + uuid.NewString()).
		SetAction("auth_provider.directory_sync").
		SetResourceType("directory_sync_job").
		SetResourceID(job.ID).
		SetActor("system:directory-enrichment-scheduler").
		SetDetails(map[string]interface{}{
			"auth_provider_id": provider.ID,
			"total_entries":    116,
			"update_count":     115,
			"blocked_count":    1,
			"error_count":      0,
		}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create directory sync audit log: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/audit-logs",
		"",
		"admin-1",
		[]string{"audit:read", "platform:admin"},
	)
	srv.ListAuditLogs(c, generated.ListAuditLogsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuditLogList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}

	item := resp.Items[0]
	if item.ResourceSummary == nil {
		t.Fatal("resource_summary = nil, want non-nil")
	}
	if item.ResourceSummary.DisplayName != "corp-directory" {
		t.Fatalf("resource_summary.display_name = %q, want corp-directory", item.ResourceSummary.DisplayName)
	}
	if item.ResourceSummary.Secondary != "scheduled_enrichment" {
		t.Fatalf("resource_summary.secondary = %q, want scheduled_enrichment", item.ResourceSummary.Secondary)
	}
	if item.ResourceSummary.Tertiary != "completed" {
		t.Fatalf("resource_summary.tertiary = %q, want completed", item.ResourceSummary.Tertiary)
	}
}
