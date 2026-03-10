package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/auditlog"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entnamespaceregistry "kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type fakeAtomicWriter struct {
	called      bool
	powerCalled bool

	ticketID     string
	eventID      string
	approver     string
	clusterID    string
	storage      string
	serviceID    string
	namespace    string
	requesterID  string
	placement    map[string]interface{}
	modifiedSpec map[string]interface{}
	powerOp      string
}

func init() {
	_ = logger.Init("error", "json")
}

func (f *fakeAtomicWriter) ApproveCreateAndEnqueue(
	_ context.Context,
	ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID string,
	_ map[string]interface{},
	_ map[string]interface{},
	placementEvaluation map[string]interface{},
	memoizedSpec map[string]interface{},
) (vmID, vmName string, err error) {
	f.called = true
	f.ticketID = ticketID
	f.eventID = eventID
	f.approver = approver
	f.clusterID = clusterID
	f.storage = storageClass
	f.serviceID = serviceID
	f.namespace = namespace
	f.requesterID = requesterID
	f.placement = placementEvaluation
	f.modifiedSpec = memoizedSpec
	return "vm-1", "vm-name", nil
}

func (f *fakeAtomicWriter) ApproveDeleteAndEnqueue(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (f *fakeAtomicWriter) ApprovePowerAndEnqueue(_ context.Context, ticketID, eventID, approver, operation string) error {
	f.powerCalled = true
	f.ticketID = ticketID
	f.eventID = eventID
	f.approver = approver
	f.powerOp = operation
	return nil
}

type dryRunProviderStub struct {
	*provider.MockProvider
	result *domain.ValidationResult
	err    error
	calls  int
}

func newDryRunProviderStub(result *domain.ValidationResult, err error) *dryRunProviderStub {
	return &dryRunProviderStub{
		MockProvider: provider.NewMockProvider(),
		result:       result,
		err:          err,
	}
}

func (s *dryRunProviderStub) ValidateSpec(_ context.Context, _, _ string, _ *domain.VMSpec) (*domain.ValidationResult, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &domain.ValidationResult{Valid: true}, nil
}

// asInt converts a JSON-decoded number (typically float64 or int) to int for assertions.
func asInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

func TestGatewayApproveCreate_CallsAtomicWriterWithResolvedIDs(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_approve")

	eventID := "event-1"
	ticketID := "ticket-1"
	requester := "user-1"
	payload := domain.VMCreationPayload{
		RequesterID:    requester,
		ServiceID:      "svc-1",
		TemplateID:     "tpl-base",
		InstanceSizeID: "size-base",
		Namespace:      "team-a",
	}
	payloadRaw, err := payload.ToJSON()
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy(requester).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create domain event: %v", err)
	}

	_, err = client.Template.Create().
		SetID("tpl-override").
		SetName("tpl").
		SetSourceType("containerdisk").
		SetImageURL("quay.io/containerdisks/ubuntu:22.04").
		SetCreatedBy("seed").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-override").
		SetName("size").
		SetCPUCores(2).
		SetMemoryGi(2).
		SetCreatedBy("seed").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	_, err = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester(requester).
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		SetModifiedSpec(map[string]interface{}{
			"template_id":      "tpl-override",
			"instance_size_id": "size-override",
		}).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	// Isolate this test to gateway orchestration; validator behavior is covered separately.
	gw.validator = nil

	if err := gw.Approve(context.Background(), ticketID, "admin-1", ApproveOpts{ClusterID: "cluster-1", StorageClass: "sc-fast"}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !writer.called {
		t.Fatal("atomic writer not called")
	}
	if writer.ticketID != ticketID || writer.eventID != eventID {
		t.Fatalf("writer ids mismatch: ticket=%s event=%s", writer.ticketID, writer.eventID)
	}
	if writer.approver != "admin-1" || writer.clusterID != "cluster-1" {
		t.Fatalf("writer approver/cluster mismatch: approver=%s cluster=%s", writer.approver, writer.clusterID)
	}
	if writer.storage != "sc-fast" {
		t.Fatalf("writer storage class = %s, want sc-fast", writer.storage)
	}
	if writer.serviceID != "svc-1" || writer.namespace != "team-a" || writer.requesterID != requester {
		t.Fatalf("writer payload mismatch: %+v", writer)
	}
	if writer.placement != nil {
		t.Fatalf("placement evaluation should be nil when validator is disabled: %#v", writer.placement)
	}
}

func TestGatewayApproveCreate_RequiresClusterSelection(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_cluster_required")

	eventID := "event-1"
	ticketID := "ticket-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"requester_id":      "user-1",
		"service_id":        "svc-1",
		"template_id":       "tpl-1",
		"instance_size_id":  "size-1",
		"namespace":         "team-a",
		"selected_cluster":  "",
		"selected_storage":  "",
		"selected_template": "",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())

	gw := NewGateway(client, nil, &fakeAtomicWriter{})
	if err := gw.Approve(context.Background(), ticketID, "admin-1", ApproveOpts{}); err == nil {
		t.Fatal("Approve() expected error when cluster id is empty, got nil")
	}
}

func TestGatewayApproveCreate_PersistsPlacementEvaluationToAuditAndAtomicWriter(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_placement_snapshot")
	ctx := context.Background()

	eventID := "event-placement"
	ticketID := "ticket-placement"
	requester := "user-1"
	payload := domain.VMCreationPayload{
		RequesterID:    requester,
		ServiceID:      "svc-1",
		TemplateID:     "tpl-1",
		InstanceSizeID: "size-1",
		Namespace:      "prod-a",
	}
	payloadRaw, err := payload.ToJSON()
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy(requester).
		Save(ctx)
	if err != nil {
		t.Fatalf("create domain event: %v", err)
	}
	_, err = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester(requester).
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(ctx)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	_, err = client.Cluster.Create().
		SetID("cluster-1").
		SetName("cluster-a").
		SetAPIServerURL("https://cluster.invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetStatus(entcluster.StatusHEALTHY).
		SetEnvironment(entcluster.EnvironmentProd).
		SetDefaultStorageClass("gold-sc").
		SetCreatedBy("seed").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	_, err = client.ClusterPolicy.Create().
		SetID("policy-1").
		SetClusterID("cluster-1").
		SetAllowCPUOvercommit(true).
		SetAllowMemoryOvercommit(true).
		SetAllowDedicatedCPU(true).
		SetAllowGpu(true).
		SetAllowSriov(true).
		SetAllowHugepages(true).
		SetAllowCdiClone(true).
		SetAllowedStorageClasses([]string{"gold-sc"}).
		SetCreatedBy("seed").
		Save(ctx)
	if err != nil {
		t.Fatalf("create cluster policy: %v", err)
	}
	_, err = client.NamespaceRegistry.Create().
		SetID("ns-1").
		SetName("prod-a").
		SetEnvironment(entnamespaceregistry.EnvironmentProd).
		SetEnabled(true).
		SetCreatedBy("seed").
		Save(ctx)
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	_, err = client.Template.Create().
		SetID("tpl-1").
		SetName("tpl").
		SetSourceType(service.TemplateSourceCDIImageImport).
		SetImageURL("docker://quay.io/containerdisks/ubuntu:22.04").
		SetCatalogScope("prod").
		SetCreatedBy("seed").
		Save(ctx)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-1").
		SetName("size").
		SetCPUCores(2).
		SetMemoryGi(2).
		SetCatalogScope("prod").
		SetCreatedBy("seed").
		Save(ctx)
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, audit.NewLogger(client), writer)

	if approveErr := gw.Approve(ctx, ticketID, "admin-1", ApproveOpts{ClusterID: "cluster-1"}); approveErr != nil {
		t.Fatalf("Approve() error = %v", approveErr)
	}
	if writer.placement == nil {
		t.Fatal("placement evaluation snapshot is nil")
	}
	if writer.placement["selected_cluster_id"] != "cluster-1" {
		t.Fatalf("placement selected_cluster_id = %v, want cluster-1", writer.placement["selected_cluster_id"])
	}
	if writer.placement["effective_storage_class"] != "gold-sc" {
		t.Fatalf("placement effective_storage_class = %v, want gold-sc", writer.placement["effective_storage_class"])
	}
	logEntry, err := client.AuditLog.Query().
		Where(auditlog.ResourceIDEQ(ticketID), auditlog.ActionEQ("approval.approved")).
		Only(ctx)
	if err != nil {
		t.Fatalf("load audit log: %v", err)
	}
	rawPlacement, ok := logEntry.Details["placement_evaluation"].(map[string]interface{})
	if !ok {
		t.Fatalf("audit placement_evaluation = %#v, want object", logEntry.Details["placement_evaluation"])
	}
	if rawPlacement["selected_cluster_id"] != "cluster-1" {
		t.Fatalf("audit selected_cluster_id = %v, want cluster-1", rawPlacement["selected_cluster_id"])
	}
}

func TestGatewayApproveVNC_TransitionsTicketAndEventWithoutAtomicWriter(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_vnc_approve")

	eventID := "event-vnc-approve-1"
	ticketID := "ticket-vnc-approve-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"vm_id":        "vm-1",
		"cluster_id":   "cluster-a",
		"namespace":    "team-a",
		"requester_id": "user-1",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVNCAccessRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-1").
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeVNC_ACCESS).
		SetReason("vnc access request").
		Save(context.Background())

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	if err := gw.Approve(context.Background(), ticketID, "admin-1", ApproveOpts{}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if writer.called {
		t.Fatal("atomic writer called for VNC ticket, want no call")
	}

	ticket, err := client.ApprovalTicket.Get(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.Status != approvalticket.StatusAPPROVED {
		t.Fatalf("ticket status = %s, want %s", ticket.Status, approvalticket.StatusAPPROVED)
	}
	if ticket.Approver != "admin-1" {
		t.Fatalf("ticket approver = %s, want admin-1", ticket.Approver)
	}

	event, err := client.DomainEvent.Get(context.Background(), eventID)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if event.Status != domainevent.StatusCOMPLETED {
		t.Fatalf("event status = %s, want %s", event.Status, domainevent.StatusCOMPLETED)
	}
}

func TestGatewayApprovePower_DelegatesToAtomicWriter(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_power_approve")

	eventID := "event-power-approve-1"
	ticketID := "ticket-power-approve-1"
	payloadRaw, _ := json.Marshal(domain.VMPowerPayload{
		VMID:      "vm-1",
		VMName:    "vm-1",
		ClusterID: "cluster-a",
		Namespace: "team-prod",
		Operation: "restart",
		Actor:     "user-1",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMRestartRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-1").
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypePOWER).
		SetReason("vm restart request").
		Save(context.Background())

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	if err := gw.Approve(context.Background(), ticketID, "admin-1", ApproveOpts{}); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !writer.powerCalled {
		t.Fatal("power atomic writer not called")
	}
	if writer.ticketID != ticketID || writer.eventID != eventID {
		t.Fatalf("writer ids mismatch: ticket=%s event=%s", writer.ticketID, writer.eventID)
	}
	if writer.approver != "admin-1" {
		t.Fatalf("writer approver = %s, want admin-1", writer.approver)
	}
	if writer.powerOp != "restart" {
		t.Fatalf("writer powerOp = %q, want %q", writer.powerOp, "restart")
	}
}

func TestGatewayReject_TransitionsTicketAndEvent(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_reject")

	eventID := "event-reject-1"
	ticketID := "ticket-reject-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"requester_id":     "user-1",
		"service_id":       "svc-1",
		"template_id":      "tpl-1",
		"instance_size_id": "size-1",
		"namespace":        "team-a",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())

	gw := NewGateway(client, nil, &fakeAtomicWriter{})
	if err := gw.Reject(context.Background(), ticketID, "admin-1", "policy mismatch"); err != nil {
		t.Fatalf("Reject() error = %v", err)
	}

	ticket, err := client.ApprovalTicket.Get(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.Status != approvalticket.StatusREJECTED {
		t.Fatalf("ticket status = %s, want %s", ticket.Status, approvalticket.StatusREJECTED)
	}
	if ticket.Approver != "admin-1" {
		t.Fatalf("ticket approver = %s, want admin-1", ticket.Approver)
	}
	event, err := client.DomainEvent.Get(context.Background(), eventID)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if event.Status != domainevent.StatusCANCELLED {
		t.Fatalf("event status = %s, want %s", event.Status, domainevent.StatusCANCELLED)
	}
}

func TestGatewayCancel_OnlyRequesterCanCancel(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_cancel")

	eventID := "event-cancel-1"
	ticketID := "ticket-cancel-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"requester_id":     "requester-1",
		"service_id":       "svc-1",
		"template_id":      "tpl-1",
		"instance_size_id": "size-1",
		"namespace":        "team-a",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("requester-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("requester-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())

	gw := NewGateway(client, nil, &fakeAtomicWriter{})
	err := gw.Cancel(context.Background(), ticketID, "other-user")
	if err == nil {
		t.Fatal("Cancel() expected forbidden error for non-requester, got nil")
	}
	if !strings.Contains(err.Error(), "only requester can cancel") {
		t.Fatalf("unexpected cancel error: %v", err)
	}
}

func TestGatewayCancel_RequesterTransitionsTicketAndEvent(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_cancel_ok")

	eventID := "event-cancel-ok-1"
	ticketID := "ticket-cancel-ok-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"requester_id":     "requester-1",
		"service_id":       "svc-1",
		"template_id":      "tpl-1",
		"instance_size_id": "size-1",
		"namespace":        "team-a",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("requester-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("requester-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())

	gw := NewGateway(client, nil, &fakeAtomicWriter{})
	if err := gw.Cancel(context.Background(), ticketID, "requester-1"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	ticket, err := client.ApprovalTicket.Get(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.Status != approvalticket.StatusCANCELLED {
		t.Fatalf("ticket status = %s, want %s", ticket.Status, approvalticket.StatusCANCELLED)
	}
	event, err := client.DomainEvent.Get(context.Background(), eventID)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if event.Status != domainevent.StatusCANCELLED {
		t.Fatalf("event status = %s, want %s", event.Status, domainevent.StatusCANCELLED)
	}
}

// --------------------------------------------------------------------------
// Resource Override Tests (Issue #1 / #2 coverage)
// --------------------------------------------------------------------------

// createOverrideTestData sets up common entities for override tests.
func createOverrideTestData(t *testing.T, client *ent.Client, suffix string) (ticketID string) {
	t.Helper()
	eventID := "event-override-" + suffix
	ticketID = "ticket-override-" + suffix
	payload := domain.VMCreationPayload{
		RequesterID:    "user-1",
		ServiceID:      "svc-1",
		TemplateID:     "tpl-override-" + suffix,
		InstanceSizeID: "size-override-" + suffix,
		Namespace:      "team-a",
	}
	payloadRaw, err := payload.ToJSON()
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create domain event: %v", err)
	}
	_, err = client.Template.Create().
		SetID("tpl-override-" + suffix).
		SetName("tpl-override-" + suffix).
		SetSourceType("containerdisk").
		SetImageURL("quay.io/containerdisks/ubuntu:22.04").
		SetCreatedBy("seed").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-override-" + suffix).
		SetName("size-override-" + suffix).
		SetCPUCores(4).
		SetMemoryGi(4).
		SetCreatedBy("seed").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}
	_, err = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	return ticketID
}

func createClonePreflightTestData(t *testing.T, client *ent.Client, suffix string) (ticketID string) {
	t.Helper()
	eventID := "event-clone-preflight-" + suffix
	ticketID = "ticket-clone-preflight-" + suffix
	payload := domain.VMCreationPayload{
		RequesterID:    "user-1",
		ServiceID:      "svc-clone-" + suffix,
		TemplateID:     "tpl-clone-" + suffix,
		InstanceSizeID: "size-clone-" + suffix,
		Namespace:      "team-a",
	}
	payloadRaw, err := payload.ToJSON()
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID(payload.ServiceID).
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create domain event: %v", err)
	}
	_, err = client.Template.Create().
		SetID(payload.TemplateID).
		SetName("tpl-clone-" + suffix).
		SetSourceType("cdi_pvc_clone").
		SetPvcName("ubuntu-golden").
		SetPvcNamespace("golden-images").
		SetCreatedBy("seed").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID(payload.InstanceSizeID).
		SetName("size-clone-" + suffix).
		SetCPUCores(2).
		SetMemoryGi(4).
		SetCreatedBy("seed").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}
	_, err = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	return ticketID
}

func TestGatewayApproveCreate_EnableOverrideAllZeros_ReturnsError(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "gateway_override_all_zeros")
	ticketID := createOverrideTestData(t, client, "all-zeros")

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil

	opts := ApproveOpts{
		ClusterID:      "cluster-1",
		EnableOverride: true,
		// All override values are zero
	}
	err := gw.Approve(context.Background(), ticketID, "admin-1", opts)
	if err == nil {
		t.Fatal("Approve() expected error when enable_override is true but all values are zero, got nil")
	}
	if !strings.Contains(err.Error(), "all resource override values are zero") {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.called {
		t.Fatal("atomic writer should not be called when override validation fails")
	}
}

func TestGatewayApproveCreate_EnableOverrideWithValues_WritesModifiedSpec(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "gateway_override_with_values")
	ticketID := createOverrideTestData(t, client, "with-values")

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil

	opts := ApproveOpts{
		ClusterID:       "cluster-1",
		EnableOverride:  true,
		CPULimit:        8,
		CPURequest:      4,
		MemoryLimitGi:   16,
		MemoryRequestGi: 8,
		DiskGB:          100,
	}
	if err := gw.Approve(context.Background(), ticketID, "admin-1", opts); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !writer.called {
		t.Fatal("atomic writer not called")
	}

	// Verify modifiedSpec contains the override values.
	ms := writer.modifiedSpec
	if ms == nil {
		t.Fatal("modifiedSpec is nil")
	}
	assertFloat64Value := func(t *testing.T, key string, expected float64) {
		t.Helper()
		v, ok := ms[key].(float64)
		if !ok {
			t.Fatalf("modifiedSpec[%q] not found or not float64: %v", key, ms[key])
		}
		if v != expected {
			t.Fatalf("modifiedSpec[%q] = %f, want %f", key, v, expected)
		}
	}
	assertFloat64Value(t, "cpu_limit", 8)
	assertFloat64Value(t, "cpu_request", 4)
	assertFloat64Value(t, "memory_limit_gi", 16)
	assertFloat64Value(t, "memory_request_gi", 8)
	assertIntValue := func(t *testing.T, key string, expected int) {
		t.Helper()
		v, ok := asInt(ms[key])
		if !ok {
			t.Fatalf("modifiedSpec[%q] not found or not int: %v", key, ms[key])
		}
		if v != expected {
			t.Fatalf("modifiedSpec[%q] = %d, want %d", key, v, expected)
		}
	}
	assertIntValue(t, "disk_gb", 100)

	if val, ok := ms["enable_override"].(bool); !ok || !val {
		t.Fatalf("modifiedSpec[enable_override] = %v, want true", ms["enable_override"])
	}
}

func TestGatewayApproveCreate_EnableOverrideDiskOnly_WorksWithoutCPUMemory(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "gateway_override_disk_only")
	ticketID := createOverrideTestData(t, client, "disk-only")

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil

	opts := ApproveOpts{
		ClusterID:      "cluster-1",
		EnableOverride: true,
		DiskGB:         200,
	}
	if err := gw.Approve(context.Background(), ticketID, "admin-1", opts); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !writer.called {
		t.Fatal("atomic writer not called")
	}
	ms := writer.modifiedSpec
	if ms == nil {
		t.Fatal("modifiedSpec is nil")
	}
	v, ok := asInt(ms["disk_gb"])
	if !ok || v != 200 {
		t.Fatalf("modifiedSpec[disk_gb] = %v, want 200", ms["disk_gb"])
	}
	if val, ok := ms["enable_override"].(bool); !ok || !val {
		t.Fatalf("modifiedSpec[enable_override] = %v, want true", ms["enable_override"])
	}
}

// --------------------------------------------------------------------------
// DryRun Pre-flight Gate Tests (ADR-0006 Addendum / P1-A)
// --------------------------------------------------------------------------

// Since service.VMService wraps provider.InfrastructureProvider, this unit test
// verifies the dry-run gate behavior through the nil-service path:
// when vmService is nil, the pre-flight gate is skipped and approval proceeds.
// Integration coverage for real VMService wiring is validated elsewhere.

func TestGatewayApproveCreate_DryRunGate_SkippedWhenVMServiceNil(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "gateway_dryrun_gate_skip")
	ticketID := createOverrideTestData(t, client, "dryrun-skip")

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil
	// vmService is nil by default — gate should be skipped, approval proceeds normally

	opts := ApproveOpts{
		ClusterID: "cluster-1",
	}
	if err := gw.Approve(context.Background(), ticketID, "admin-1", opts); err != nil {
		t.Fatalf("Approve() error = %v; expected success when vmService is nil (gate skipped)", err)
	}
	if !writer.called {
		t.Fatal("atomic writer should have been called when DryRun gate is skipped")
	}
}

func TestGatewayApproveCreate_CloneSourcePreflight_BlocksMissingSourcePVC(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_clone_preflight_missing_source")
	ticketID := createClonePreflightTestData(t, client, "missing")

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil
	gw.SetVMService(service.NewVMService(provider.NewMockProvider()))

	err := gw.Approve(context.Background(), ticketID, "admin-1", ApproveOpts{ClusterID: "cluster-1", StorageClass: "fast-sc"})
	if err == nil {
		t.Fatal("Approve() expected clone source preflight error, got nil")
	}
	if writer.called {
		t.Fatal("atomic writer must NOT be called when clone source preflight fails")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("Approve() error = %T, want wrapped AppError", err)
	}
	if appErr.Code != "PVC_CLONE_SOURCE_NOT_FOUND" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_SOURCE_NOT_FOUND")
	}
}

func TestGatewayApproveCreate_CloneSourcePreflight_BlocksTargetDiskSmallerThanSource(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_clone_preflight_target_too_small")
	ticketID := createClonePreflightTestData(t, client, "target-too-small")
	if _, err := client.InstanceSize.UpdateOneID("size-clone-target-too-small").SetDiskGB(20).Save(context.Background()); err != nil {
		t.Fatalf("set instance size disk_gb: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:                  "ubuntu-golden",
		Namespace:             "golden-images",
		Phase:                 "Bound",
		RequestedStorageBytes: 40 * 1024 * 1024 * 1024,
	}})

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil
	gw.SetVMService(service.NewVMService(mock))

	err := gw.Approve(context.Background(), ticketID, "admin-1", ApproveOpts{ClusterID: "cluster-1", StorageClass: "fast-sc"})
	if err == nil {
		t.Fatal("Approve() expected clone target size preflight error, got nil")
	}
	if writer.called {
		t.Fatal("atomic writer must NOT be called when clone target size preflight fails")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("Approve() error = %T, want wrapped AppError", err)
	}
	if appErr.Code != "PVC_CLONE_TARGET_TOO_SMALL" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_TARGET_TOO_SMALL")
	}
}

func TestGatewayApproveCreate_CloneSourcePreflight_BlocksExpansionOnNonExpandableStorageClass(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_clone_preflight_expansion_unsupported")
	ticketID := createClonePreflightTestData(t, client, "expansion-unsupported")
	if _, err := client.InstanceSize.UpdateOneID("size-clone-expansion-unsupported").SetDiskGB(40).Save(context.Background()); err != nil {
		t.Fatalf("set instance size disk_gb: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:                  "ubuntu-golden",
		Namespace:             "golden-images",
		Phase:                 "Bound",
		RequestedStorageBytes: 20 * 1024 * 1024 * 1024,
	}})
	mock.SeedStorageClasses([]*domain.StorageClass{{
		Name:                 "slow-sc",
		AllowVolumeExpansion: false,
	}})

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil
	gw.SetVMService(service.NewVMService(mock))

	err := gw.Approve(context.Background(), ticketID, "admin-1", ApproveOpts{ClusterID: "cluster-1", StorageClass: "slow-sc"})
	if err == nil {
		t.Fatal("Approve() expected clone expansion storage class error, got nil")
	}
	if writer.called {
		t.Fatal("atomic writer must NOT be called when clone expansion preflight fails")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("Approve() error = %T, want wrapped AppError", err)
	}
	if appErr.Code != "PVC_CLONE_TARGET_EXPANSION_UNSUPPORTED" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_TARGET_EXPANSION_UNSUPPORTED")
	}
}

func TestGatewayApproveCreate_CloneSourcePreflight_BlocksMissingTargetStorageClass(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_clone_preflight_missing_target_storage_class")
	ticketID := createClonePreflightTestData(t, client, "missing-target-sc")
	if _, err := client.InstanceSize.UpdateOneID("size-clone-missing-target-sc").SetDiskGB(20).Save(context.Background()); err != nil {
		t.Fatalf("set instance size disk_gb: %v", err)
	}

	mock := provider.NewMockProvider()
	mock.SeedPVCs([]*domain.PersistentVolumeClaim{{
		Name:                  "ubuntu-golden",
		Namespace:             "golden-images",
		Phase:                 "Bound",
		RequestedStorageBytes: 20 * 1024 * 1024 * 1024,
	}})

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil
	gw.SetVMService(service.NewVMService(mock))

	err := gw.Approve(context.Background(), ticketID, "admin-1", ApproveOpts{ClusterID: "cluster-1", StorageClass: "missing-sc"})
	if err == nil {
		t.Fatal("Approve() expected missing target storage class error, got nil")
	}
	if writer.called {
		t.Fatal("atomic writer must NOT be called when clone target storage class preflight fails")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("Approve() error = %T, want wrapped AppError", err)
	}
	if appErr.Code != "PVC_CLONE_TARGET_STORAGE_CLASS_NOT_FOUND" {
		t.Fatalf("AppError.Code = %q, want %q", appErr.Code, "PVC_CLONE_TARGET_STORAGE_CLASS_NOT_FOUND")
	}
}

func TestGatewayApproveCreate_DryRunGate_ValidSpecCallsAtomicWriter(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "gateway_dryrun_gate_valid")
	ticketID := createOverrideTestData(t, client, "dryrun-valid")

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil
	stubProvider := newDryRunProviderStub(&domain.ValidationResult{Valid: true}, nil)
	gw.SetVMService(service.NewVMService(stubProvider))

	opts := ApproveOpts{ClusterID: "cluster-1"}
	if err := gw.Approve(context.Background(), ticketID, "admin-1", opts); err != nil {
		t.Fatalf("Approve() error = %v; expected success when DryRun gate passes", err)
	}
	if !writer.called {
		t.Fatal("atomic writer should have been called when DryRun gate passes")
	}
	if stubProvider.calls != 1 {
		t.Fatalf("ValidateSpec calls = %d, want 1", stubProvider.calls)
	}
}

func TestGatewayApproveCreate_DryRunGate_InvalidSpecBlocksEnqueue(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "gateway_dryrun_gate_invalid")
	ticketID := createOverrideTestData(t, client, "dryrun-invalid")

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil
	stubProvider := newDryRunProviderStub(&domain.ValidationResult{
		Valid:  false,
		Errors: []string{"cpu limit exceeds quota"},
	}, nil)
	gw.SetVMService(service.NewVMService(stubProvider))

	opts := ApproveOpts{ClusterID: "cluster-1"}
	err := gw.Approve(context.Background(), ticketID, "admin-1", opts)
	if err == nil {
		t.Fatal("Approve() expected error when DryRun gate rejects spec, got nil")
	}
	if !strings.Contains(err.Error(), "vm spec rejected by cluster") {
		t.Fatalf("Approve() error = %v, want dryrun rejection context", err)
	}
	if writer.called {
		t.Fatal("atomic writer must NOT be called when DryRun gate rejects spec")
	}
	if stubProvider.calls != 1 {
		t.Fatalf("ValidateSpec calls = %d, want 1", stubProvider.calls)
	}
}

func TestGatewayApproveCreate_DryRunGate_ProviderErrorBlocksEnqueue(t *testing.T) {
	t.Parallel()
	client := testutil.OpenEntPostgres(t, "gateway_dryrun_gate_error")
	ticketID := createOverrideTestData(t, client, "dryrun-error")

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	gw.validator = nil
	stubProvider := newDryRunProviderStub(nil, fmt.Errorf("apiserver unreachable"))
	gw.SetVMService(service.NewVMService(stubProvider))

	opts := ApproveOpts{ClusterID: "cluster-1"}
	err := gw.Approve(context.Background(), ticketID, "admin-1", opts)
	if err == nil {
		t.Fatal("Approve() expected error when DryRun gate provider fails, got nil")
	}
	if !strings.Contains(err.Error(), "pre-flight dryrun gate") {
		t.Fatalf("Approve() error = %v, want pre-flight dryrun gate context", err)
	}
	if writer.called {
		t.Fatal("atomic writer must NOT be called when DryRun gate provider fails")
	}
	if stubProvider.calls != 1 {
		t.Fatalf("ValidateSpec calls = %d, want 1", stubProvider.calls)
	}
}

func TestGatewaySetVMService_IsChainable(t *testing.T) {
	t.Parallel()
	// Verify SetVMService returns *Gateway for chaining.
	gw := NewGateway(nil, nil, nil)
	returned := gw.SetVMService(nil)
	if returned != gw {
		t.Fatal("SetVMService() should return the same Gateway for method chaining")
	}
}

func TestBuildDryRunSpec_AppliesOverrideValues(t *testing.T) {
	t.Parallel()

	// Create minimal Ent Template and InstanceSize stubs for spec building.
	tmpl := &ent.Template{
		ID:         "tpl-test",
		SourceType: "containerdisk",
		ImageURL:   "quay.io/containerdisks/ubuntu:22.04",
	}
	size := &ent.InstanceSize{
		ID:       "size-test",
		CPUCores: 2.0,
		MemoryGi: 4.0,
		DiskGB:   50,
	}
	payload := &vmCreatePayload{ServiceID: "svc-1", Namespace: "team-a"}
	gw := &Gateway{}

	t.Run("base_values_without_override", func(t *testing.T) {
		t.Parallel()
		spec, err := gw.buildDryRunSpec(payload, tmpl, size, ApproveOpts{})
		if err != nil {
			t.Fatalf("buildDryRunSpec() error = %v", err)
		}
		if spec.CPU != 2.0 {
			t.Errorf("spec.CPU = %f, want 2.0", spec.CPU)
		}
		if spec.MemoryGi != 4.0 {
			t.Errorf("spec.MemoryGi = %f, want 4.0", spec.MemoryGi)
		}
		if spec.DiskGB != 50 {
			t.Errorf("spec.DiskGB = %d, want 50", spec.DiskGB)
		}
		if spec.Image != "quay.io/containerdisks/ubuntu:22.04" {
			t.Errorf("spec.Image = %q, want quay.io/containerdisks/ubuntu:22.04", spec.Image)
		}
		// DryRun name must use "dryrun-" prefix
		if !strings.HasPrefix(spec.Name, "dryrun-") {
			t.Errorf("spec.Name = %q, want prefix 'dryrun-'", spec.Name)
		}
	})

	t.Run("admin_resource_override_applied", func(t *testing.T) {
		t.Parallel()
		opts := ApproveOpts{
			EnableOverride:  true,
			CPULimit:        8.0,
			MemoryLimitGi:   16.0,
			CPURequest:      4.0,
			MemoryRequestGi: 8.0,
			DiskGB:          200,
		}
		spec, err := gw.buildDryRunSpec(payload, tmpl, size, opts)
		if err != nil {
			t.Fatalf("buildDryRunSpec() with override error = %v", err)
		}
		if spec.CPU != 8.0 {
			t.Errorf("spec.CPU = %f, want 8.0 (override)", spec.CPU)
		}
		if spec.MemoryGi != 16.0 {
			t.Errorf("spec.MemoryGi = %f, want 16.0 (override)", spec.MemoryGi)
		}
		if spec.DiskGB != 200 {
			t.Errorf("spec.DiskGB = %d, want 200 (override)", spec.DiskGB)
		}
	})

	t.Run("disk_only_override", func(t *testing.T) {
		t.Parallel()
		opts := ApproveOpts{DiskGB: 100}
		spec, err := gw.buildDryRunSpec(payload, tmpl, size, opts)
		if err != nil {
			t.Fatalf("buildDryRunSpec() disk-only override error = %v", err)
		}
		if spec.DiskGB != 100 {
			t.Errorf("spec.DiskGB = %d, want 100 (disk-only override)", spec.DiskGB)
		}
		// CPU/Memory unchanged
		if spec.CPU != 2.0 {
			t.Errorf("spec.CPU = %f, want 2.0 (unchanged)", spec.CPU)
		}
	})
}

func TestResolveTemplateImageForDryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tmpl      *ent.Template
		wantImage string
		wantErr   bool
	}{
		{
			name:      "image_url",
			tmpl:      &ent.Template{ID: "t1", SourceType: "containerdisk", ImageURL: "quay.io/ubuntu:22.04"},
			wantImage: "quay.io/ubuntu:22.04",
		},
		{
			name:      "pvc_name_only",
			tmpl:      &ent.Template{ID: "t2", SourceType: "cdi_pvc_clone", PvcName: "golden-image"},
			wantImage: "clone-pvc:golden-image",
		},
		{
			name:      "pvc_name_with_namespace",
			tmpl:      &ent.Template{ID: "t3", SourceType: "cdi_pvc_clone", PvcName: "golden-image", PvcNamespace: "images"},
			wantImage: "clone-pvc:images/golden-image",
		},
		{
			name:      "cdi_image_import",
			tmpl:      &ent.Template{ID: "t4", SourceType: "cdi_image_import", ImageURL: "quay.io/ubuntu:22.04"},
			wantImage: "import-image:docker://quay.io/ubuntu:22.04",
		},
		{
			name:    "no_image_source",
			tmpl:    &ent.Template{ID: "t5"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveTemplateImageForDryRun(tc.tmpl)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveTemplateImageForDryRun() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTemplateImageForDryRun() error = %v", err)
			}
			if got != tc.wantImage {
				t.Errorf("resolveTemplateImageForDryRun() = %q, want %q", got, tc.wantImage)
			}
		})
	}
}
