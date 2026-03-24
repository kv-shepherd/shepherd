package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
)

func TestDeleteNamespace_RejectsActiveCreateRequests(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	ns, err := client.NamespaceRegistry.Create().
		SetID("ns-active-req").
		SetName("prod-active-req").
		SetEnvironment(namespaceregistry.EnvironmentProd).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	payload, err := json.Marshal(domain.VMCreationPayload{
		RequesterID:    "user-a",
		ServiceID:      "svc-a",
		TemplateID:     "tpl-a",
		InstanceSizeID: "size-a",
		Namespace:      ns.Name,
		Reason:         "pending request",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := client.DomainEvent.Create().
		SetID("ev-ns-active-req").
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-a").
		SetPayload(payload).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("user-a").
		Save(ctx); err != nil {
		t.Fatalf("create domain event: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID("ticket-ns-active-req").
		SetEventID("ev-ns-active-req").
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusPENDING).
		SetRequester("user-a").
		SetReason("pending request").
		Save(ctx); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/namespaces/"+ns.ID+"?confirm_name="+ns.Name,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteNamespace(c, ns.ID, generated.DeleteNamespaceParams{ConfirmName: ns.Name})
	if w.Code != http.StatusConflict {
		t.Fatalf("delete namespace status = %d, want %d, body=%s", w.Code, http.StatusConflict, w.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != "NAMESPACE_HAS_ACTIVE_REQUESTS" {
		t.Fatalf("error code = %q, want NAMESPACE_HAS_ACTIVE_REQUESTS", resp.Code)
	}
}
