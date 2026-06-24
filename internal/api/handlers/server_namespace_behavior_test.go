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

func TestListNamespaces_FiltersBySearchAcrossNameDescriptionAndActor(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	createNamespace := func(id, name, description, createdBy string) {
		t.Helper()
		builder := client.NamespaceRegistry.Create().
			SetID(id).
			SetName(name).
			SetEnvironment(namespaceregistry.EnvironmentTest).
			SetEnabled(true).
			SetCreatedBy(createdBy)
		if description != "" {
			builder = builder.SetDescription(description)
		}
		if _, err := builder.Save(ctx); err != nil {
			t.Fatalf("create namespace %s: %v", id, err)
		}
	}

	createNamespace("ns-1", "finance-core", "finance workloads", "alice")
	createNamespace("ns-2", "platform-core", "platform workloads", "bob")

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/namespaces?page=1&per_page=20&search=finance",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListNamespaces(c, generated.ListNamespacesParams{
		Page:    1,
		PerPage: 20,
		Search:  "finance",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list namespaces status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.NamespaceRegistryList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].Name != "finance-core" {
		t.Fatalf("items[0].name = %q, want finance-core", resp.Items[0].Name)
	}
}

func TestListNamespaces_FiltersByEnabledState(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	createNamespace := func(id, name string, enabled bool) {
		t.Helper()
		_, err := client.NamespaceRegistry.Create().
			SetID(id).
			SetName(name).
			SetEnvironment(namespaceregistry.EnvironmentTest).
			SetEnabled(enabled).
			SetCreatedBy("alice").
			Save(ctx)
		if err != nil {
			t.Fatalf("create namespace %s: %v", id, err)
		}
	}

	createNamespace("ns-enabled", "team-enabled", true)
	createNamespace("ns-disabled", "team-disabled", false)

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/namespaces?page=1&per_page=20&enabled=true",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListNamespaces(c, generated.ListNamespacesParams{
		Page:    1,
		PerPage: 20,
		Enabled: true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("list namespaces status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.NamespaceRegistryList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if resp.Items[0].Name != "team-enabled" {
		t.Fatalf("items[0].name = %q, want team-enabled", resp.Items[0].Name)
	}
}

func TestListNamespaces_RejectsInvalidEnvironment(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminIdentityTestServer(t)

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/namespaces?environment=stage",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListNamespaces(c, generated.ListNamespacesParams{
		Environment: generated.ListNamespacesParamsEnvironment("stage"),
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}
