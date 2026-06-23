package usecase

import (
	"encoding/json"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestNewDeleteVMUseCaseAndWithAuditLogger(t *testing.T) {
	t.Parallel()

	uc := NewDeleteVMUseCase(nil)
	if uc == nil {
		t.Fatal("NewDeleteVMUseCase() returned nil")
	}
	if uc.entClient != nil {
		t.Fatalf("entClient = %#v, want nil", uc.entClient)
	}

	auditLogger := &audit.Logger{}
	got := uc.WithAuditLogger(auditLogger)
	if got != uc {
		t.Fatal("WithAuditLogger() should return the same use case pointer")
	}
	if uc.auditLogger != auditLogger {
		t.Fatal("WithAuditLogger() did not store the audit logger")
	}
}

func TestDeleteVMUseCaseExecuteCreatesDeleteRequest(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "delete_vm_execute_success")
	uc := NewDeleteVMUseCase(client)
	fixture := seedDeleteVMUseCaseFixture(t, client, "success", namespaceregistry.EnvironmentProd, entvm.StatusSTOPPED)

	out, err := uc.Execute(t.Context(), DeleteVMInput{
		VMID:        fixture.VMID,
		ConfirmName: "  " + fixture.VMName + "  ",
		RequestedBy: "delete-requester",
	})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("Execute() returned nil output")
	}
	if out.Status != "PENDING" {
		t.Fatalf("output status = %q, want PENDING", out.Status)
	}
	if out.EventID == "" || out.TicketID == "" {
		t.Fatalf("output should include event and ticket IDs: %+v", out)
	}

	event, err := client.DomainEvent.Get(t.Context(), out.EventID)
	if err != nil {
		t.Fatalf("load delete event: %v", err)
	}
	if event.EventType != string(domain.EventVMDeletionRequested) {
		t.Fatalf("event type = %q, want %q", event.EventType, domain.EventVMDeletionRequested)
	}
	if event.AggregateType != "vm" || event.AggregateID != fixture.VMID {
		t.Fatalf("event aggregate = (%q, %q), want (vm, %s)", event.AggregateType, event.AggregateID, fixture.VMID)
	}
	if event.Status != domainevent.StatusPENDING {
		t.Fatalf("event status = %q, want PENDING", event.Status)
	}
	if event.CreatedBy != "delete-requester" {
		t.Fatalf("event created_by = %q, want delete-requester", event.CreatedBy)
	}

	var payload domain.VMDeletePayload
	if unmarshalErr := json.Unmarshal(event.Payload, &payload); unmarshalErr != nil {
		t.Fatalf("unmarshal delete payload: %v", unmarshalErr)
	}
	if payload.VMID != fixture.VMID || payload.VMName != fixture.VMName {
		t.Fatalf("payload VM identity mismatch: %#v", payload)
	}
	if payload.ClusterID != fixture.ClusterID || payload.ClusterName != fixture.ClusterName {
		t.Fatalf("payload cluster mismatch: %#v", payload)
	}
	if payload.ClusterEnvironment != string(namespaceregistry.EnvironmentProd) {
		t.Fatalf("payload cluster environment = %q, want prod", payload.ClusterEnvironment)
	}
	if payload.Namespace != fixture.Namespace {
		t.Fatalf("payload namespace = %q, want %q", payload.Namespace, fixture.Namespace)
	}
	if payload.SystemID != fixture.SystemID || payload.SystemName != fixture.SystemName {
		t.Fatalf("payload system mismatch: %#v", payload)
	}
	if payload.ServiceID != fixture.ServiceID || payload.ServiceName != fixture.ServiceName {
		t.Fatalf("payload service mismatch: %#v", payload)
	}
	if payload.OwnerID != fixture.OwnerID || payload.OwnerDisplayName != fixture.OwnerDisplayName || payload.OwnerUsername != fixture.OwnerUsername {
		t.Fatalf("payload owner mismatch: %#v", payload)
	}
	if payload.TemplateID != fixture.TemplateID || payload.TemplateName != fixture.TemplateName {
		t.Fatalf("payload template snapshot mismatch: %#v", payload)
	}
	if payload.InstanceSizeID != fixture.InstanceSizeID || payload.InstanceSizeName != fixture.InstanceSizeName {
		t.Fatalf("payload instance size snapshot mismatch: %#v", payload)
	}
	if payload.CurrentCPUCores != 4 || payload.CurrentMemoryGi != 8 || payload.CurrentDiskGB != 120 {
		t.Fatalf("payload current resource snapshot mismatch: %#v", payload)
	}
	if payload.RequestVMStatus != string(entvm.StatusSTOPPED) {
		t.Fatalf("payload request_vm_status = %q, want STOPPED", payload.RequestVMStatus)
	}
	if payload.Actor != "delete-requester" {
		t.Fatalf("payload actor = %q, want delete-requester", payload.Actor)
	}

	ticket, err := client.Ticket.Get(t.Context(), out.TicketID)
	if err != nil {
		t.Fatalf("load delete ticket: %v", err)
	}
	if ticket.EventID != out.EventID {
		t.Fatalf("ticket event_id = %q, want %q", ticket.EventID, out.EventID)
	}
	if ticket.OperationType != entticket.OperationTypeDELETE {
		t.Fatalf("ticket operation = %q, want DELETE", ticket.OperationType)
	}
	if ticket.Status != entticket.StatusPENDING {
		t.Fatalf("ticket status = %q, want PENDING", ticket.Status)
	}
	if ticket.Requester != "delete-requester" {
		t.Fatalf("ticket requester = %q, want delete-requester", ticket.Requester)
	}
	wantReason := "Request to delete VM " + fixture.VMName
	if ticket.Reason != wantReason {
		t.Fatalf("ticket reason = %q, want %q", ticket.Reason, wantReason)
	}
}

func TestDeleteVMUseCaseExecuteRejectsInvalidRequests(t *testing.T) {
	testCases := []struct {
		name        string
		setup       func(t *testing.T, client *ent.Client) DeleteVMInput
		wantErrCode string
	}{
		{
			name: "missing VM",
			setup: func(t *testing.T, client *ent.Client) DeleteVMInput {
				t.Helper()
				return DeleteVMInput{VMID: "missing-vm", Confirm: true, RequestedBy: "delete-requester"}
			},
			wantErrCode: apperrors.CodeVMNotFound,
		},
		{
			name: "test namespace requires confirm flag",
			setup: func(t *testing.T, client *ent.Client) DeleteVMInput {
				t.Helper()
				fixture := seedDeleteVMUseCaseFixture(t, client, "confirm", namespaceregistry.EnvironmentTest, entvm.StatusSTOPPED)
				return DeleteVMInput{VMID: fixture.VMID, RequestedBy: "delete-requester"}
			},
			wantErrCode: "DELETE_CONFIRMATION_REQUIRED",
		},
		{
			name: "running VM is not deletable",
			setup: func(t *testing.T, client *ent.Client) DeleteVMInput {
				t.Helper()
				fixture := seedDeleteVMUseCaseFixture(t, client, "running", namespaceregistry.EnvironmentTest, entvm.StatusRUNNING)
				return DeleteVMInput{VMID: fixture.VMID, Confirm: true, RequestedBy: "delete-requester"}
			},
			wantErrCode: VMDeleteInvalidStateCode,
		},
		{
			name: "duplicate pending delete request",
			setup: func(t *testing.T, client *ent.Client) DeleteVMInput {
				t.Helper()
				fixture := seedDeleteVMUseCaseFixture(t, client, "duplicate", namespaceregistry.EnvironmentTest, entvm.StatusSTOPPED)
				seedPendingDeleteTicket(t, client, fixture.VMID, "duplicate")
				return DeleteVMInput{VMID: fixture.VMID, Confirm: true, RequestedBy: "delete-requester"}
			},
			wantErrCode: apperrors.CodeDuplicateRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.OpenEntPostgres(t, "delete_vm_execute_"+tc.name)
			uc := NewDeleteVMUseCase(client)
			input := tc.setup(t, client)

			_, err := uc.Execute(t.Context(), input)
			if err == nil {
				t.Fatalf("Execute() error = nil, want code %s", tc.wantErrCode)
			}
			appErr, ok := apperrors.IsAppError(err)
			if !ok {
				t.Fatalf("expected AppError, got %T: %v", err, err)
			}
			if appErr.Code != tc.wantErrCode {
				t.Fatalf("error code = %q, want %q", appErr.Code, tc.wantErrCode)
			}
		})
	}
}

type deleteVMUseCaseFixture struct {
	VMID             string
	VMName           string
	Namespace        string
	SystemID         string
	SystemName       string
	ServiceID        string
	ServiceName      string
	ClusterID        string
	ClusterName      string
	OwnerID          string
	OwnerDisplayName string
	OwnerUsername    string
	TemplateID       string
	TemplateName     string
	InstanceSizeID   string
	InstanceSizeName string
}

func seedDeleteVMUseCaseFixture(
	t *testing.T,
	client *ent.Client,
	suffix string,
	environment namespaceregistry.Environment,
	status entvm.Status,
) deleteVMUseCaseFixture {
	t.Helper()

	ctx := t.Context()
	namespace := "team-" + suffix
	ownerID := "owner-" + suffix
	systemID := "system-" + suffix
	serviceID := "service-" + suffix
	clusterID := "cluster-" + suffix
	vmID := "vm-" + suffix
	vmName := "vm-" + suffix
	createEventID := "event-create-" + suffix
	createTicketID := "ticket-create-" + suffix
	templateID := "template-" + suffix
	instanceSizeID := "size-" + suffix

	if _, err := client.NamespaceRegistry.Create().
		SetID("namespace-" + suffix).
		SetName(namespace).
		SetEnvironment(environment).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("seed namespace: %v", err)
	}
	owner, err := client.User.Create().
		SetID(ownerID).
		SetUsername("alice-" + suffix).
		SetDisplayName("Alice Owner").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	system, err := client.System.Create().
		SetID(systemID).
		SetName("payments").
		SetCreatedBy(owner.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed system: %v", err)
	}
	service, err := client.Service.Create().
		SetID(serviceID).
		SetName("billing").
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed service: %v", err)
	}

	clusterEnv := entcluster.EnvironmentTest
	if environment == namespaceregistry.EnvironmentProd {
		clusterEnv = entcluster.EnvironmentProd
	}
	cluster, err := client.Cluster.Create().
		SetID(clusterID).
		SetName(clusterID).
		SetDisplayName("Cluster " + suffix).
		SetAPIServerURL("https://" + clusterID + ".invalid").
		SetEncryptedKubeconfig([]byte("apiVersion: v1\nkind: Config\n")).
		SetCreatedBy("seed").
		SetEnvironment(clusterEnv).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed cluster: %v", err)
	}

	createPayloadBytes, err := domain.VMCreationPayload{
		RequesterID:      owner.ID,
		OwnerID:          owner.ID,
		ServiceID:        service.ID,
		ServiceName:      service.Name,
		SystemID:         system.ID,
		SystemName:       system.Name,
		TemplateID:       templateID,
		TemplateName:     "Ubuntu 22.04",
		InstanceSizeID:   instanceSizeID,
		InstanceSizeName: "M4 Large",
		Namespace:        namespace,
		Reason:           "seed VM",
		OwnerDisplayName: owner.DisplayName,
		OwnerUsername:    owner.Username,
		TargetCPUCores:   4,
		TargetMemoryGi:   8,
		TargetDiskGB:     120,
	}.ToJSON()
	if err != nil {
		t.Fatalf("marshal create payload: %v", err)
	}
	if _, err := client.DomainEvent.Create().
		SetID(createEventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("service").
		SetAggregateID(service.ID).
		SetPayload(createPayloadBytes).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy(owner.ID).
		Save(ctx); err != nil {
		t.Fatalf("seed create event: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID(createTicketID).
		SetEventID(createEventID).
		SetRequester(owner.ID).
		SetStatus(entticket.StatusSUCCESS).
		SetOperationType(entticket.OperationTypeCREATE).
		SetReason("seed VM").
		Save(ctx); err != nil {
		t.Fatalf("seed create ticket: %v", err)
	}
	if _, err := client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace(namespace).
		SetClusterID(cluster.ID).
		SetStatus(status).
		SetCreatedBy(owner.ID).
		SetTicketID(createTicketID).
		SetServiceID(service.ID).
		Save(ctx); err != nil {
		t.Fatalf("seed VM: %v", err)
	}

	return deleteVMUseCaseFixture{
		VMID:             vmID,
		VMName:           vmName,
		Namespace:        namespace,
		SystemID:         system.ID,
		SystemName:       system.Name,
		ServiceID:        service.ID,
		ServiceName:      service.Name,
		ClusterID:        cluster.ID,
		ClusterName:      cluster.DisplayName,
		OwnerID:          owner.ID,
		OwnerDisplayName: owner.DisplayName,
		OwnerUsername:    owner.Username,
		TemplateID:       templateID,
		TemplateName:     "Ubuntu 22.04",
		InstanceSizeID:   instanceSizeID,
		InstanceSizeName: "M4 Large",
	}
}

func seedPendingDeleteTicket(t *testing.T, client *ent.Client, vmID, suffix string) {
	t.Helper()

	payloadBytes, err := domain.VMDeletePayload{
		VMID:  vmID,
		Actor: "seed",
	}.ToJSON()
	if err != nil {
		t.Fatalf("marshal delete payload: %v", err)
	}
	eventID := "event-delete-" + suffix
	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMDeletionRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(t.Context()); err != nil {
		t.Fatalf("seed pending delete event: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID("ticket-delete-" + suffix).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("duplicate").
		Save(t.Context()); err != nil {
		t.Fatalf("seed pending delete ticket: %v", err)
	}
}

func TestValidateDeleteConfirmationByEnvironment(t *testing.T) {
	testCases := []struct {
		name        string
		environment namespaceregistry.Environment
		confirm     bool
		confirmName string
		wantErrCode string
	}{
		{
			name:        "test env accepts confirm true",
			environment: namespaceregistry.EnvironmentTest,
			confirm:     true,
		},
		{
			name:        "test env rejects confirm_name only",
			environment: namespaceregistry.EnvironmentTest,
			confirmName: "vm-01",
			wantErrCode: "DELETE_CONFIRMATION_REQUIRED",
		},
		{
			name:        "prod env requires confirm_name",
			environment: namespaceregistry.EnvironmentProd,
			confirm:     true,
			wantErrCode: "DELETE_CONFIRMATION_REQUIRED",
		},
		{
			name:        "prod env rejects mismatched confirm_name",
			environment: namespaceregistry.EnvironmentProd,
			confirmName: "other-vm",
			wantErrCode: "CONFIRMATION_NAME_MISMATCH",
		},
		{
			name:        "prod env accepts exact confirm_name",
			environment: namespaceregistry.EnvironmentProd,
			confirmName: "vm-01",
		},
		{
			name:        "prod env trims confirm_name whitespace",
			environment: namespaceregistry.EnvironmentProd,
			confirmName: "  vm-01  ",
		},
		{
			name:        "unsupported environment rejected",
			environment: namespaceregistry.Environment("staging"),
			confirm:     true,
			wantErrCode: "UNSUPPORTED_NAMESPACE_ENVIRONMENT",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDeleteConfirmationByEnvironment("vm-01", tc.environment, tc.confirm, tc.confirmName)
			if tc.wantErrCode == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error code %s, got nil", tc.wantErrCode)
			}
			appErr, ok := apperrors.IsAppError(err)
			if !ok {
				t.Fatalf("expected AppError, got %T", err)
			}
			if appErr.Code != tc.wantErrCode {
				t.Fatalf("error code mismatch: got %s want %s", appErr.Code, tc.wantErrCode)
			}
		})
	}
}

func TestDeleteVMUseCaseResolveNamespaceEnvironment(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "delete_vm_namespace_env")
	uc := NewDeleteVMUseCase(client)

	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-test").
		SetName("team-test").
		SetEnvironment(namespaceregistry.EnvironmentTest).
		SetCreatedBy("seed").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed enabled namespace: %v", err)
	}
	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-disabled").
		SetName("team-disabled").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy("seed").
		SetEnabled(false).
		Save(t.Context()); err != nil {
		t.Fatalf("seed disabled namespace: %v", err)
	}

	testCases := []struct {
		name        string
		namespace   string
		wantEnv     namespaceregistry.Environment
		wantErrCode string
	}{
		{
			name:      "enabled namespace resolves after trim",
			namespace: " team-test ",
			wantEnv:   namespaceregistry.EnvironmentTest,
		},
		{
			name:        "empty namespace rejected before query",
			namespace:   " ",
			wantErrCode: "NAMESPACE_REQUIRED",
		},
		{
			name:        "disabled namespace is unavailable",
			namespace:   "team-disabled",
			wantErrCode: "NAMESPACE_ENVIRONMENT_NOT_FOUND",
		},
		{
			name:        "missing namespace is unavailable",
			namespace:   "missing",
			wantErrCode: "NAMESPACE_ENVIRONMENT_NOT_FOUND",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := uc.resolveNamespaceEnvironment(t.Context(), tc.namespace)
			if tc.wantErrCode == "" {
				if err != nil {
					t.Fatalf("resolveNamespaceEnvironment() unexpected error: %v", err)
				}
				if env != tc.wantEnv {
					t.Fatalf("resolveNamespaceEnvironment() = %q, want %q", env, tc.wantEnv)
				}
				return
			}

			if err == nil {
				t.Fatalf("resolveNamespaceEnvironment() error = nil, want code %s", tc.wantErrCode)
			}
			appErr, ok := apperrors.IsAppError(err)
			if !ok {
				t.Fatalf("expected AppError, got %T", err)
			}
			if appErr.Code != tc.wantErrCode {
				t.Fatalf("error code = %q, want %q", appErr.Code, tc.wantErrCode)
			}
		})
	}
}

func TestValidateDeleteConfirmationByEnvironment_ProdRejectsMissingSignal(t *testing.T) {
	t.Parallel()

	err := validateDeleteConfirmationByEnvironment("vm-01", namespaceregistry.EnvironmentProd, false, "")
	if err == nil {
		t.Fatal("expected DELETE_CONFIRMATION_REQUIRED, got nil")
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != "DELETE_CONFIRMATION_REQUIRED" {
		t.Fatalf("error code mismatch: got %s want DELETE_CONFIRMATION_REQUIRED", appErr.Code)
	}
}

func TestVMDeleteAllowedStatusAndErrorPayload(t *testing.T) {
	t.Parallel()

	allowed := []entvm.Status{
		entvm.StatusSTOPPED,
		entvm.StatusFAILED,
		entvm.StatusNOT_FOUND,
		entvm.StatusUNKNOWN,
	}
	for _, status := range allowed {
		if !VMDeleteAllowedStatus(status) {
			t.Fatalf("VMDeleteAllowedStatus(%q) = false, want true", status)
		}
	}

	if VMDeleteAllowedStatus(entvm.StatusRUNNING) {
		t.Fatal("VMDeleteAllowedStatus(RUNNING) = true, want false")
	}

	message := VMDeleteInvalidStateMessage(entvm.StatusRUNNING)
	if !strings.Contains(message, string(entvm.StatusRUNNING)) {
		t.Fatalf("VMDeleteInvalidStateMessage() = %q, want current status", message)
	}
	if !strings.Contains(message, VMDeleteAllowedStatesLabel()) {
		t.Fatalf("VMDeleteInvalidStateMessage() = %q, want allowed states label", message)
	}

	params := VMDeleteInvalidStateParams(entvm.StatusRUNNING)
	if got := params["current_state"]; got != string(entvm.StatusRUNNING) {
		t.Fatalf("params[current_state] = %#v, want %q", got, entvm.StatusRUNNING)
	}
	if got := params["allowed_states"]; got != VMDeleteAllowedStatesLabel() {
		t.Fatalf("params[allowed_states] = %#v, want %q", got, VMDeleteAllowedStatesLabel())
	}
}

func TestDeleteVMUseCaseFindPendingDeleteDuplicate(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "delete_vm_duplicate")
	uc := NewDeleteVMUseCase(client)
	ctx := t.Context()
	vmID := "vm-delete-1"

	mustCreateDeleteEvent := func(t *testing.T, eventID string, aggregateID string, payload domain.VMDeletePayload) {
		t.Helper()
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		if _, err := client.DomainEvent.Create().
			SetID(eventID).
			SetEventType(string(domain.EventVMDeletionRequested)).
			SetAggregateType("vm").
			SetAggregateID(aggregateID).
			SetPayload(payloadBytes).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy("seed").
			Save(ctx); err != nil {
			t.Fatalf("seed domain event %s: %v", eventID, err)
		}
	}

	if _, err := client.DomainEvent.Create().
		SetID("event-malformed").
		SetEventType(string(domain.EventVMDeletionRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload([]byte(`{"vm_id":`)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("seed").
		Save(ctx); err != nil {
		t.Fatalf("seed malformed event: %v", err)
	}
	mustCreateDeleteEvent(t, "event-other-payload", vmID, domain.VMDeletePayload{VMID: "other-vm"})
	mustCreateDeleteEvent(t, "event-without-ticket", vmID, domain.VMDeletePayload{VMID: vmID})
	mustCreateDeleteEvent(t, "event-approved-ticket", vmID, domain.VMDeletePayload{VMID: vmID})
	if _, err := client.Ticket.Create().
		SetID("ticket-approved").
		SetEventID("event-approved-ticket").
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("already handled").
		Save(ctx); err != nil {
		t.Fatalf("seed approved ticket: %v", err)
	}
	mustCreateDeleteEvent(t, "event-pending-ticket", vmID, domain.VMDeletePayload{VMID: vmID})
	if _, err := client.Ticket.Create().
		SetID("ticket-pending").
		SetEventID("event-pending-ticket").
		SetRequester("seed").
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationTypeDELETE).
		SetReason("duplicate").
		Save(ctx); err != nil {
		t.Fatalf("seed pending ticket: %v", err)
	}

	ticket, err := uc.findPendingDeleteDuplicate(ctx, " "+vmID+" ")
	if err != nil {
		t.Fatalf("findPendingDeleteDuplicate() unexpected error: %v", err)
	}
	if ticket == nil {
		t.Fatal("findPendingDeleteDuplicate() returned nil, want pending ticket")
	}
	if ticket.ID != "ticket-pending" {
		t.Fatalf("ticket.ID = %q, want ticket-pending", ticket.ID)
	}

	none, err := uc.findPendingDeleteDuplicate(ctx, "vm-without-duplicate")
	if err != nil {
		t.Fatalf("findPendingDeleteDuplicate(non-duplicate) unexpected error: %v", err)
	}
	if none != nil {
		t.Fatalf("findPendingDeleteDuplicate(non-duplicate) = %#v, want nil", none)
	}
}

func TestApplyDeleteCreationSnapshot(t *testing.T) {
	t.Parallel()

	payload := domain.VMDeletePayload{
		ServiceName:      "payments-api",
		SystemID:         "",
		SystemName:       "",
		OwnerID:          "",
		OwnerDisplayName: "",
		OwnerUsername:    "",
	}
	createPayload := domain.VMCreationPayload{
		SystemID:         "system-1",
		SystemName:       "Payments",
		ServiceID:        "service-1",
		ServiceName:      "billing-worker",
		OwnerID:          "user-1",
		OwnerDisplayName: "Alex Chen",
		OwnerUsername:    "alexchen",
		TemplateID:       "template-openeuler",
		TemplateName:     "OpenEuler 22.03",
		InstanceSizeID:   "size-m4",
		InstanceSizeName: "M4 Large",
		TargetCPUCores:   4,
		TargetMemoryGi:   8,
		TargetDiskGB:     60,
	}

	applyDeleteCreationSnapshot(&payload, createPayload)

	if payload.TemplateID != "template-openeuler" || payload.TemplateName != "OpenEuler 22.03" {
		t.Fatalf("template snapshot not applied: %#v", payload)
	}
	if payload.InstanceSizeID != "size-m4" || payload.InstanceSizeName != "M4 Large" {
		t.Fatalf("instance size snapshot not applied: %#v", payload)
	}
	if payload.CurrentCPUCores != 4 || payload.CurrentMemoryGi != 8 || payload.CurrentDiskGB != 60 {
		t.Fatalf("resource snapshot not applied: %#v", payload)
	}
	if payload.SystemID != "system-1" || payload.SystemName != "Payments" {
		t.Fatalf("system snapshot not applied: %#v", payload)
	}
	if payload.ServiceName != "payments-api" {
		t.Fatalf("existing service name should win, got %#v", payload.ServiceName)
	}
	if payload.OwnerDisplayName != "Alex Chen" || payload.OwnerUsername != "alexchen" {
		t.Fatalf("owner snapshot not applied: %#v", payload)
	}
}
