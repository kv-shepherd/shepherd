package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	rrb "kv-shepherd.io/shepherd/ent/resourcerolebinding"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestSystemHandler_ListSystems_RespectsResourceBindings(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)

	mustCreateUserForSystemSearch(t, client, "owner-1", "owner-1@example.com", "Owner One")
	sysVisible := mustCreateSystem(t, client, "sys-visible", "shop", "owner-1")
	_ = mustCreateSystem(t, client, "sys-hidden", "finance", "owner-2")
	mustCreateSystemBinding(t, client, "user-a", sysVisible.ID, "viewer")

	c, w := newAuthedGinContext(t, http.MethodGet, "/systems", "", "user-a", []string{"system:read"})
	srv.ListSystems(c, generated.ListSystemsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.SystemList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != sysVisible.ID {
		t.Fatalf("visible system id = %s, want %s", resp.Items[0].Id, sysVisible.ID)
	}
	if resp.Items[0].CreatedByDisplayName != "Owner One" {
		t.Fatalf("created_by_display_name = %#v, want Owner One", resp.Items[0].CreatedByDisplayName)
	}
	if resp.Items[0].CreatedByUsername != "owner-1@example.com" {
		t.Fatalf("created_by_username = %#v, want owner-1@example.com", resp.Items[0].CreatedByUsername)
	}
}

func TestSystemHandler_ListSystems_RejectsInvalidSortOrder(t *testing.T) {
	srv, _ := newSystemBehaviorTestServer(t)

	c, w := newAuthedGinContext(t, http.MethodGet, "/systems?sort_order=sideways", "", "platform-admin", []string{"system:read", "platform:admin"})
	srv.ListSystems(c, generated.ListSystemsParams{
		SortOrder: generated.ListSystemsParamsSortOrder("sideways"),
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestSystemHandler_ListSystems_RejectsInvalidSortBy(t *testing.T) {
	srv, _ := newSystemBehaviorTestServer(t)

	c, w := newAuthedGinContext(t, http.MethodGet, "/systems?sort_by=tenant", "", "platform-admin", []string{"system:read", "platform:admin"})
	srv.ListSystems(c, generated.ListSystemsParams{
		SortBy: "tenant",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestSystemHandler_ListSystems_SortsByCreatedAtAscendingWhenRequested(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	ctx := t.Context()

	olderCreatedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	newerCreatedAt := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	_, err := client.System.Create().
		SetID("sys-old-" + uuid.NewString()).
		SetName("oldsys").
		SetCreatedBy("platform-admin").
		SetCreatedAt(olderCreatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create older system: %v", err)
	}
	newer, err := client.System.Create().
		SetID("sys-new-" + uuid.NewString()).
		SetName("newsys").
		SetCreatedBy("platform-admin").
		SetCreatedAt(newerCreatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create newer system: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/systems?sort_order=asc", "", "platform-admin", []string{"system:read", "platform:admin"})
	srv.ListSystems(c, generated.ListSystemsParams{
		SortOrder: generated.ListSystemsParamsSortOrderAsc,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.SystemList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if len(resp.Items) < 2 {
		t.Fatalf("items len = %d, want at least 2", len(resp.Items))
	}
	if resp.Items[0].Name != "oldsys" {
		t.Fatalf("first system name = %q, want oldsys", resp.Items[0].Name)
	}
	if resp.Items[1].Id != newer.ID {
		t.Fatalf("second system id = %q, want %q", resp.Items[1].Id, newer.ID)
	}
}

func TestSystemHandler_ListSystems_SortsByNameWhenRequested(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	ctx := t.Context()

	_, err := client.System.Create().
		SetID("sys-z-" + uuid.NewString()).
		SetName("zsort").
		SetCreatedBy("platform-admin").
		SetCreatedAt(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create z system: %v", err)
	}
	firstByName, err := client.System.Create().
		SetID("sys-a-" + uuid.NewString()).
		SetName("asort").
		SetCreatedBy("platform-admin").
		SetCreatedAt(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create a system: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/systems?sort_by=name&sort_order=asc", "", "platform-admin", []string{"system:read", "platform:admin"})
	srv.ListSystems(c, generated.ListSystemsParams{
		SortBy:    "name",
		SortOrder: generated.ListSystemsParamsSortOrderAsc,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.SystemList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if len(resp.Items) < 2 {
		t.Fatalf("items len = %d, want at least 2", len(resp.Items))
	}
	if resp.Items[0].Id != firstByName.ID {
		t.Fatalf("first system id = %q, want %q", resp.Items[0].Id, firstByName.ID)
	}
}

func TestSystemHandler_ListSystems_SupportsSearchAcrossSystemCreatorServiceAndMember(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)

	mustCreateUserForSystemSearch(t, client, "user-alice", "alice.ops", "Alice Ops")
	sysMatch := mustCreateSystem(t, client, "sys-platform", "platform core", "user-alice")
	sysOther := mustCreateSystem(t, client, "sys-finance", "finance ops", "charlie.ops")
	mustCreateService(t, client, "svc-billing", "billing-api", sysMatch.ID, "Handles partner billing")
	mustCreateService(t, client, "svc-ledger", "ledger-api", sysOther.ID, "Handles accounting")
	mustCreateUserForSystemSearch(t, client, "user-bob", "bob.builder@example.com", "Bob Builder")
	mustCreateUserForSystemSearch(t, client, "user-dana", "dana.lee@example.com", "Dana Lee")
	mustCreateSystemBinding(t, client, "user-bob", sysMatch.ID, "viewer")
	mustCreateSystemBinding(t, client, "user-dana", sysOther.ID, "viewer")

	assertListSystems := func(t *testing.T, rawQuery string, params generated.ListSystemsParams, wantIDs ...string) {
		t.Helper()

		c, w := newAuthedGinContext(
			t,
			http.MethodGet,
			"/systems"+rawQuery,
			"",
			"platform-admin",
			[]string{"system:read", "platform:admin"},
		)
		srv.ListSystems(c, params)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp generated.SystemList
		mustDecodeJSON(t, w.Body.Bytes(), &resp)
		if got := len(resp.Items); got != len(wantIDs) {
			t.Fatalf("items len = %d, want %d", got, len(wantIDs))
		}
		for idx, wantID := range wantIDs {
			if got := resp.Items[idx].Id; got != wantID {
				t.Fatalf("items[%d].id = %q, want %q", idx, got, wantID)
			}
		}
	}

	t.Run("quick search matches creator", func(t *testing.T) {
		assertListSystems(
			t,
			"?search=alice",
			generated.ListSystemsParams{Search: "alice"},
			sysMatch.ID,
		)
	})

	t.Run("quick search matches related service", func(t *testing.T) {
		assertListSystems(
			t,
			"?search=billing",
			generated.ListSystemsParams{Search: "billing"},
			sysMatch.ID,
		)
	})

	t.Run("quick search matches related member", func(t *testing.T) {
		assertListSystems(
			t,
			"?search=builder",
			generated.ListSystemsParams{Search: "builder"},
			sysMatch.ID,
		)
	})

	t.Run("advanced filters narrow by creator service and member", func(t *testing.T) {
		assertListSystems(
			t,
			"?created_by=alice&service_search=billing&member_search=bob",
			generated.ListSystemsParams{
				CreatedBy:     "alice",
				ServiceSearch: "billing",
				MemberSearch:  "bob",
			},
			sysMatch.ID,
		)
	})

	t.Run("exact filters narrow by creator service id and member id", func(t *testing.T) {
		assertListSystems(
			t,
			"?created_by_exact=user-alice&service_id=svc-billing&member_id=user-bob",
			generated.ListSystemsParams{
				CreatedByExact: "user-alice",
				ServiceId:      "svc-billing",
				MemberId:       "user-bob",
			},
			sysMatch.ID,
		)
	})
}

func TestSystemHandler_GetSystemFilterOptions_ReturnsReadableVisibleOptions(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)

	sysVisible := mustCreateSystem(t, client, "sys-visible", "shop", "alice.ops")
	sysHidden := mustCreateSystem(t, client, "sys-hidden", "finance", "charlie.ops")
	_ = mustCreateService(t, client, "svc-visible", "billing-api", sysVisible.ID, "Handles partner billing")
	_ = mustCreateService(t, client, "svc-hidden", "ledger-api", sysHidden.ID, "Handles accounting")
	mustCreateUserForSystemSearch(t, client, "user-a", "alice.ops", "Alice Ops")
	mustCreateUserForSystemSearch(t, client, "user-bob", "bob.builder@example.com", "Bob Builder")
	mustCreateUserForSystemSearch(t, client, "user-dana", "dana.lee@example.com", "Dana Lee")
	mustCreateSystemBinding(t, client, "user-a", sysVisible.ID, "viewer")
	mustCreateSystemBinding(t, client, "user-bob", sysVisible.ID, "viewer")
	mustCreateSystemBinding(t, client, "user-dana", sysHidden.ID, "viewer")

	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/filter-options", "", "user-a", []string{"system:read"})
	srv.GetSystemFilterOptions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.SystemFilterOptionsResponse
	mustDecodeJSON(t, w.Body.Bytes(), &resp)

	if len(resp.Creators) != 1 {
		t.Fatalf("creators len = %d, want 1", len(resp.Creators))
	}
	if got := resp.Creators[0].Label; got != "Alice Ops · alice.ops" {
		t.Fatalf("creator label = %q, want %q", got, "Alice Ops · alice.ops")
	}
	if got := resp.Creators[0].Value; got != "alice.ops" {
		t.Fatalf("creator value = %q, want %q", got, "alice.ops")
	}

	if len(resp.Services) != 1 {
		t.Fatalf("services len = %d, want 1", len(resp.Services))
	}
	if got := resp.Services[0].Label; got != "shop / billing-api" {
		t.Fatalf("service label = %q, want %q", got, "shop / billing-api")
	}
	if got := resp.Services[0].Value; got != "svc-visible" {
		t.Fatalf("service value = %q, want %q", got, "svc-visible")
	}

	if len(resp.Members) != 2 {
		t.Fatalf("members len = %d, want 2", len(resp.Members))
	}
	memberLabels := map[string]struct{}{}
	for _, option := range resp.Members {
		memberLabels[option.Label] = struct{}{}
	}
	if _, ok := memberLabels["Bob Builder · bob.builder@example.com"]; !ok {
		t.Fatalf("member labels = %#v, want Bob Builder option", memberLabels)
	}
	if _, ok := memberLabels["Alice Ops · alice.ops"]; !ok {
		t.Fatalf("member labels = %#v, want Alice Ops option", memberLabels)
	}
}

func TestSystemHandler_ListServicesOverview_RespectsVisibilityAndFilter(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)

	sysVisibleA := mustCreateSystem(t, client, "sys-visible-a", "shop", "owner-1")
	sysVisibleB := mustCreateSystem(t, client, "sys-visible-b", "payments", "owner-1")
	sysHidden := mustCreateSystem(t, client, "sys-hidden", "finance", "owner-2")

	svcVisibleA := mustCreateService(t, client, "svc-visible-a", "redis", sysVisibleA.ID, "cache")
	_ = mustCreateService(t, client, "svc-visible-b", "api", sysVisibleB.ID, "backend")
	_ = mustCreateService(t, client, "svc-hidden", "db", sysHidden.ID, "database")

	mustCreateSystemBinding(t, client, "user-a", sysVisibleA.ID, "viewer")
	mustCreateSystemBinding(t, client, "user-a", sysVisibleB.ID, "viewer")

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/services?page=1&per_page=20",
		"",
		"user-a",
		[]string{"service:read"},
	)
	srv.ListServicesOverview(c, generated.ListServicesOverviewParams{Page: 1, PerPage: 20})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ServiceList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 2 {
		t.Fatalf("items len = %d, want 2", got)
	}

	visibleSystems := map[string]string{}
	for _, item := range resp.Items {
		visibleSystems[item.Id] = item.SystemName
	}
	if got := visibleSystems[svcVisibleA.ID]; got != sysVisibleA.Name {
		t.Fatalf("service %s system_name = %q, want %q", svcVisibleA.ID, got, sysVisibleA.Name)
	}
	if _, ok := visibleSystems["svc-hidden"]; ok {
		t.Fatal("hidden service unexpectedly visible in overview")
	}

	filteredContext, filteredWriter := newAuthedGinContext(
		t,
		http.MethodGet,
		"/services?page=1&per_page=20&system_id="+sysVisibleB.ID,
		"",
		"user-a",
		[]string{"service:read"},
	)
	srv.ListServicesOverview(filteredContext, generated.ListServicesOverviewParams{
		Page:     1,
		PerPage:  20,
		SystemId: sysVisibleB.ID,
	})

	if filteredWriter.Code != http.StatusOK {
		t.Fatalf("filtered status = %d, want %d body=%s", filteredWriter.Code, http.StatusOK, filteredWriter.Body.String())
	}
	mustDecodeJSON(t, filteredWriter.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("filtered items len = %d, want 1", got)
	}
	if got := resp.Items[0].SystemId; got != sysVisibleB.ID {
		t.Fatalf("filtered system_id = %q, want %q", got, sysVisibleB.ID)
	}

	searchContext, searchWriter := newAuthedGinContext(
		t,
		http.MethodGet,
		"/services?page=1&per_page=20&search=payments",
		"",
		"user-a",
		[]string{"service:read"},
	)
	srv.ListServicesOverview(searchContext, generated.ListServicesOverviewParams{
		Page:    1,
		PerPage: 20,
		Search:  "payments",
	})

	if searchWriter.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d body=%s", searchWriter.Code, http.StatusOK, searchWriter.Body.String())
	}
	mustDecodeJSON(t, searchWriter.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("search items len = %d, want 1", got)
	}
	if got := resp.Items[0].SystemId; got != sysVisibleB.ID {
		t.Fatalf("search system_id = %q, want %q", got, sysVisibleB.ID)
	}
}

func TestSystemHandler_ListServices_FiltersWithinSystem(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)

	sys := mustCreateSystem(t, client, "sys-services", "shop", "owner-1")
	otherSys := mustCreateSystem(t, client, "sys-other-services", "other", "owner-1")
	apiSvc := mustCreateService(t, client, "svc-api", "api", sys.ID, "backend")
	_ = mustCreateService(t, client, "svc-cache", "cache", sys.ID, "redis")
	_ = mustCreateService(t, client, "svc-other-api", "api", otherSys.ID, "other backend")
	mustCreateSystemBinding(t, client, "user-a", sys.ID, "viewer")

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/systems/"+sys.ID+"/services?page=1&per_page=20&search=api",
		"",
		"user-a",
		[]string{"service:read"},
	)
	srv.ListServices(c, sys.ID, generated.ListServicesParams{
		Page:    1,
		PerPage: 20,
		Search:  "api",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ServiceList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1", got)
	}
	if got := resp.Items[0].Id; got != apiSvc.ID {
		t.Fatalf("service id = %q, want %q", got, apiSvc.ID)
	}
	if got := resp.Items[0].SystemId; got != sys.ID {
		t.Fatalf("system_id = %q, want %q", got, sys.ID)
	}
}

func TestSystemHandler_GetService_IncludesSystemName(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-1", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-1", "redis", sys.ID, "old")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "viewer")

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/systems/"+sys.ID+"/services/"+svc.ID,
		"",
		"owner-1",
		[]string{"service:read"},
	)
	srv.GetService(c, sys.ID, svc.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.Service
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.SystemName != sys.Name {
		t.Fatalf("system_name = %q, want %q", resp.SystemName, sys.Name)
	}
}

func TestSystemHandler_GetServiceWorkspaceContext_ShapesServiceVMsAndOwnRequests(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-1", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-1", "redis", sys.ID, "service workspace")
	otherSvc := mustCreateService(t, client, "svc-2", "api", sys.ID, "other service")
	mustCreateVMForService(t, client, "vm-1", "shop-redis-01", svc.ID)
	mustCreateVMForService(t, client, "vm-2", "shop-api-01", otherSvc.ID)
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "viewer")

	makeCreateTicket := func(ticketID, eventID, requester, aggregateID string) {
		t.Helper()

		payload, err := json.Marshal(domain.VMCreationPayload{
			RequesterID:    requester,
			ServiceID:      aggregateID,
			TemplateID:     uuid.NewString(),
			InstanceSizeID: uuid.NewString(),
			Namespace:      "prod-shop",
			Reason:         "Need a VM",
		})
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		if _, err := client.DomainEvent.Create().
			SetID(eventID).
			SetEventType(string(domain.EventVMCreationRequested)).
			SetAggregateType("service").
			SetAggregateID(aggregateID).
			SetPayload(payload).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy(requester).
			Save(t.Context()); err != nil {
			t.Fatalf("create domain event: %v", err)
		}
		if _, err := client.Ticket.Create().
			SetID(ticketID).
			SetEventID(eventID).
			SetOperationType(entticket.OperationTypeCREATE).
			SetStatus(entticket.StatusPENDING).
			SetRequester(requester).
			SetReason("Need a VM").
			Save(t.Context()); err != nil {
			t.Fatalf("create ticket: %v", err)
		}
	}

	makeCreateTicket("ticket-own", "event-own", "owner-1", svc.ID)
	makeCreateTicket("ticket-other-user", "event-other-user", "owner-2", svc.ID)
	makeCreateTicket("ticket-other-service", "event-other-service", "owner-1", otherSvc.ID)

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/systems/"+sys.ID+"/services/"+svc.ID+"/context",
		"",
		"owner-1",
		[]string{"service:read", "vm:read", "platform:admin"},
	)
	srv.GetServiceWorkspaceContext(c, sys.ID, svc.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ServiceWorkspaceContext
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := resp.Service.SystemName; got != sys.Name {
		t.Fatalf("service.system_name = %q, want %q", got, sys.Name)
	}
	if got := resp.Summary.VisibleVmCount; got != 1 {
		t.Fatalf("summary.visible_vm_count = %d, want 1", got)
	}
	if got := resp.Summary.RecentRequestCount; got != 1 {
		t.Fatalf("summary.recent_request_count = %d, want 1", got)
	}
	if got := len(resp.VisibleVms); got != 1 {
		t.Fatalf("visible_vms len = %d, want 1", got)
	}
	if got := resp.VisibleVms[0].ServiceId; got != svc.ID {
		t.Fatalf("visible_vms[0].service_id = %q, want %q", got, svc.ID)
	}
	if got := len(resp.RecentRequests); got != 1 {
		t.Fatalf("recent_requests len = %d, want 1", got)
	}
	if got := resp.RecentRequests[0].Requester; got != "owner-1" {
		t.Fatalf("recent_requests[0].requester = %q, want owner-1", got)
	}
}

func TestSystemHandler_UpdateSystem_DescriptionOnly(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-1", "shop", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+sys.ID,
		`{"description":"new description"}`,
		"owner-1",
		[]string{"system:write"},
	)
	srv.UpdateSystem(c, sys.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	updated, err := client.System.Get(c.Request.Context(), sys.ID)
	if err != nil {
		t.Fatalf("query updated system: %v", err)
	}
	if updated.Name != "shop" {
		t.Fatalf("system name changed unexpectedly: got %q want %q", updated.Name, "shop")
	}
	if updated.Description != "new description" {
		t.Fatalf("description = %q, want %q", updated.Description, "new description")
	}
}

func TestSystemHandler_UpdateSystem_ForbiddenWithoutSystemRole(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-1", "shop", "owner-1")

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+sys.ID,
		`{"description":"new description"}`,
		"user-no-role",
		[]string{"system:write"},
	)
	srv.UpdateSystem(c, sys.ID)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestSystemHandler_UpdateService_DescriptionOnly(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-1", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-1", "redis", sys.ID, "old")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+sys.ID+"/services/"+svc.ID,
		`{"description":"service updated"}`,
		"owner-1",
		[]string{"service:create"},
	)
	srv.UpdateService(c, sys.ID, svc.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	updated, err := client.Service.Get(c.Request.Context(), svc.ID)
	if err != nil {
		t.Fatalf("query updated service: %v", err)
	}
	if updated.Name != "redis" {
		t.Fatalf("service name changed unexpectedly: got %q want %q", updated.Name, "redis")
	}
	if updated.Description != "service updated" {
		t.Fatalf("service description = %q, want %q", updated.Description, "service updated")
	}
}

func TestSystemHandler_UpdateService_NotFoundWhenSystemMismatch(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sysA := mustCreateSystem(t, client, "sys-a", "shop", "owner-a")
	sysB := mustCreateSystem(t, client, "sys-b", "finance", "owner-b")
	svc := mustCreateService(t, client, "svc-1", "redis", sysB.ID, "old")
	mustCreateSystemBinding(t, client, "owner-a", sysA.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+sysA.ID+"/services/"+svc.ID,
		`{"description":"service updated"}`,
		"owner-a",
		[]string{"service:create"},
	)
	srv.UpdateService(c, sysA.ID, svc.ID)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestSystemHandler_DeleteSystem_RequiresConfirmNameMatch(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c1, w1 := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID,
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(c1, sys.ID, generated.DeleteSystemParams{})
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w1.Code, http.StatusBadRequest, w1.Body.String())
	}
	assertErrorCode(t, w1.Body.Bytes(), "DELETE_CONFIRMATION_REQUIRED")

	c2, w2 := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"?confirm_name=wrong",
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(c2, sys.ID, generated.DeleteSystemParams{ConfirmName: "wrong"})
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w2.Code, http.StatusBadRequest, w2.Body.String())
	}
	assertErrorCode(t, w2.Body.Bytes(), "DELETE_CONFIRMATION_REQUIRED")
}

func TestSystemHandler_DeleteSystem_ConflictWhenServicesExist(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	_ = mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"?confirm_name=shop",
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(c, sys.ID, generated.DeleteSystemParams{ConfirmName: "shop"})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "SYSTEM_HAS_SERVICES")
}

func TestSystemHandler_DeleteSystem_Success(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"?confirm_name=shop",
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(c, sys.ID, generated.DeleteSystemParams{ConfirmName: "shop"})
	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
	}

	if _, err := client.System.Get(t.Context(), sys.ID); !ent.IsNotFound(err) {
		t.Fatalf("system still exists after delete, err=%v", err)
	}
	bindingCount, err := client.ResourceRoleBinding.Query().
		Where(
			rrb.ResourceTypeEQ("system"),
			rrb.ResourceIDEQ(sys.ID),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count system bindings after delete: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("system bindings after delete = %d, want 0", bindingCount)
	}
}

func TestSystemHandler_DeleteSystemWinsBeforeMemberAddWithoutOrphan(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	systemRow := mustCreateSystem(t, client, "sys-delete-member-race", "delrace", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", systemRow.ID, "owner")
	if _, err := client.User.Create().
		SetID("member-delete-race").
		SetUsername("member.delete.race").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create member target: %v", err)
	}

	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemRow.ID+"?confirm_name="+systemRow.Name,
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	addContext, addResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems/"+systemRow.ID+"/members",
		`{"user_id":"member-delete-race","role":"viewer"}`,
		"owner-1",
		[]string{"rbac:manage", "platform:admin"},
	)

	releaseGuard, blockerPID := holdSystemMembershipGuard(t, srv.pool, systemRow.ID)
	deleteDone := runHandlerAsync(func() {
		srv.DeleteSystem(deleteContext, systemRow.ID, generated.DeleteSystemParams{ConfirmName: systemRow.Name})
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	addDone := runHandlerAsync(func() {
		srv.AddSystemMember(addContext, systemRow.ID)
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	releaseGuard()
	waitForHandlerCompletion(t, deleteDone, "delete system before member add")
	waitForHandlerCompletion(t, addDone, "member add after system delete")

	if deleteContext.Writer.Status() != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d body=%s", deleteContext.Writer.Status(), http.StatusNoContent, deleteResponse.Body.String())
	}
	if addResponse.Code != http.StatusNotFound {
		t.Fatalf("member add status = %d, want %d body=%s", addResponse.Code, http.StatusNotFound, addResponse.Body.String())
	}
	assertErrorCode(t, addResponse.Body.Bytes(), "SYSTEM_NOT_FOUND")
	if _, err := client.System.Get(t.Context(), systemRow.ID); !ent.IsNotFound(err) {
		t.Fatalf("system lookup error = %v, want not found", err)
	}
	bindingCount, err := client.ResourceRoleBinding.Query().
		Where(
			rrb.ResourceTypeEQ("system"),
			rrb.ResourceIDEQ(systemRow.ID),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count bindings after delete/add race: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("bindings after delete/add race = %d, want 0", bindingCount)
	}
}

func TestSystemHandler_DeleteSystemRechecksActorRoleAfterConcurrentRemoval(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	systemRow := mustCreateSystem(t, client, "sys-delete-auth-race", "authrace", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", systemRow.ID, "owner")
	mustCreateSystemBinding(t, client, "owner-2", systemRow.ID, "owner")

	removeContext, removeResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemRow.ID+"/members/owner-1",
		"",
		"platform-admin",
		[]string{"rbac:manage", "platform:admin"},
	)
	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemRow.ID+"?confirm_name="+systemRow.Name,
		"",
		"owner-1",
		[]string{"system:delete"},
	)

	releaseGuard, blockerPID := holdSystemMembershipGuard(t, srv.pool, systemRow.ID)
	removeDone := runHandlerAsync(func() {
		srv.DeleteSystemMember(removeContext, systemRow.ID, "owner-1")
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	deleteDone := runHandlerAsync(func() {
		srv.DeleteSystem(deleteContext, systemRow.ID, generated.DeleteSystemParams{ConfirmName: systemRow.Name})
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	releaseGuard()
	waitForHandlerCompletion(t, removeDone, "remove deleting actor membership")
	waitForHandlerCompletion(t, deleteDone, "delete system after actor removal")

	if removeContext.Writer.Status() != http.StatusNoContent {
		t.Fatalf("member removal status = %d, want %d body=%s", removeContext.Writer.Status(), http.StatusNoContent, removeResponse.Body.String())
	}
	if deleteResponse.Code != http.StatusForbidden {
		t.Fatalf("system delete status = %d, want %d body=%s", deleteResponse.Code, http.StatusForbidden, deleteResponse.Body.String())
	}
	assertErrorCode(t, deleteResponse.Body.Bytes(), "FORBIDDEN")
	if _, err := client.System.Get(t.Context(), systemRow.ID); err != nil {
		t.Fatalf("system should remain after actor permission loss: %v", err)
	}
	actorBindingExists, err := client.ResourceRoleBinding.Query().
		Where(
			rrb.ResourceTypeEQ("system"),
			rrb.ResourceIDEQ(systemRow.ID),
			rrb.UserIDEQ("owner-1"),
		).
		Exist(t.Context())
	if err != nil {
		t.Fatalf("check removed actor binding: %v", err)
	}
	if actorBindingExists {
		t.Fatal("removed actor binding still exists")
	}
}

func TestSystemHandler_DeleteSystemBeforeOwnerUserDeleteAllowsBothDeletes(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	if _, err := client.User.Create().
		SetID("owner-delete-race").
		SetUsername("owner.delete.race").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	systemRow := mustCreateSystem(t, client, "sys-delete-owner-race", "ownerrace", "owner-delete-race")
	mustCreateSystemBinding(t, client, "owner-delete-race", systemRow.ID, "owner")
	mustCreateSystemBinding(t, client, "owner-remaining", systemRow.ID, "owner")

	deleteSystemContext, deleteSystemResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemRow.ID+"?confirm_name="+systemRow.Name,
		"",
		"owner-delete-race",
		[]string{"system:delete"},
	)
	deleteUserContext, deleteUserResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/owner-delete-race",
		"",
		"platform-admin",
		[]string{"user:manage", "platform:admin"},
	)

	releaseGuard, blockerPID := holdSystemMembershipGuard(t, srv.pool, systemRow.ID)
	deleteSystemDone := runHandlerAsync(func() {
		srv.DeleteSystem(deleteSystemContext, systemRow.ID, generated.DeleteSystemParams{ConfirmName: systemRow.Name})
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	deleteUserDone := runHandlerAsync(func() {
		srv.DeleteUser(deleteUserContext, "owner-delete-race")
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	releaseGuard()
	waitForHandlerCompletion(t, deleteSystemDone, "delete system before owner user")
	waitForHandlerCompletion(t, deleteUserDone, "delete owner user after system")

	if deleteSystemContext.Writer.Status() != http.StatusNoContent {
		t.Fatalf("system delete status = %d, want %d body=%s", deleteSystemContext.Writer.Status(), http.StatusNoContent, deleteSystemResponse.Body.String())
	}
	if deleteUserContext.Writer.Status() != http.StatusNoContent {
		t.Fatalf("user delete status = %d, want %d body=%s", deleteUserContext.Writer.Status(), http.StatusNoContent, deleteUserResponse.Body.String())
	}
	if _, err := client.System.Get(t.Context(), systemRow.ID); !ent.IsNotFound(err) {
		t.Fatalf("system lookup error = %v, want not found", err)
	}
	if _, err := client.User.Get(t.Context(), "owner-delete-race"); !ent.IsNotFound(err) {
		t.Fatalf("owner user lookup error = %v, want not found", err)
	}
}

func TestSystemHandler_DeleteSystemRollsBackBindingCleanupOnDeleteFailure(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	systemRow := mustCreateSystem(t, client, "sys-delete-rollback", "rollback", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", systemRow.ID, "owner")

	if _, err := srv.pool.Exec(t.Context(), `
CREATE OR REPLACE FUNCTION fail_system_delete_for_test()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'forced system delete failure';
END;
$$;
CREATE TRIGGER fail_system_delete_for_test
BEFORE DELETE ON systems
FOR EACH ROW
EXECUTE FUNCTION fail_system_delete_for_test();
`); err != nil {
		t.Fatalf("install system delete failure trigger: %v", err)
	}

	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemRow.ID+"?confirm_name="+systemRow.Name,
		"",
		"owner-1",
		[]string{"system:delete"},
	)
	srv.DeleteSystem(deleteContext, systemRow.ID, generated.DeleteSystemParams{ConfirmName: systemRow.Name})
	if deleteResponse.Code != http.StatusInternalServerError {
		t.Fatalf("delete status = %d, want %d body=%s", deleteResponse.Code, http.StatusInternalServerError, deleteResponse.Body.String())
	}
	if _, err := client.System.Get(t.Context(), systemRow.ID); err != nil {
		t.Fatalf("system should remain after rollback: %v", err)
	}
	bindingCount, err := client.ResourceRoleBinding.Query().
		Where(
			rrb.ResourceTypeEQ("system"),
			rrb.ResourceIDEQ(systemRow.ID),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count bindings after rollback: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("bindings after rollback = %d, want 1", bindingCount)
	}
}

func TestSystemHandler_DeleteService_RequiresConfirmTrue(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"/services/"+svc.ID,
		"",
		"owner-1",
		[]string{"service:delete"},
	)
	srv.DeleteService(c, sys.ID, svc.ID, generated.DeleteServiceParams{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "DELETE_CONFIRMATION_REQUIRED")
}

func TestSystemHandler_DeleteService_ConflictWhenVMsExist(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateVMForService(t, client, "vm-del-1", "shop-redis-01", svc.ID)
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"/services/"+svc.ID+"?confirm=true",
		"",
		"owner-1",
		[]string{"service:delete"},
	)
	srv.DeleteService(c, sys.ID, svc.ID, generated.DeleteServiceParams{Confirm: true})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "SERVICE_HAS_VMS")

	var resp generated.Error
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := resp.Params["vm_count"]; got != float64(1) {
		t.Fatalf("vm_count = %#v, want 1", got)
	}
}

func TestSystemHandler_DeleteService_Success(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"/services/"+svc.ID+"?confirm=true",
		"",
		"owner-1",
		[]string{"service:delete"},
	)
	srv.DeleteService(c, sys.ID, svc.ID, generated.DeleteServiceParams{Confirm: true})
	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
	}

	if _, err := client.Service.Get(t.Context(), svc.ID); !ent.IsNotFound(err) {
		t.Fatalf("service still exists after delete, err=%v", err)
	}
}

func TestSystemHandler_DeleteService_ConflictWhenActiveCreateRequestsExist(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	sys := mustCreateSystem(t, client, "sys-del", "shop", "owner-1")
	svc := mustCreateService(t, client, "svc-del", "redis", sys.ID, "svc")
	mustCreateSystemBinding(t, client, "owner-1", sys.ID, "owner")

	payload, err := json.Marshal(domain.VMCreationPayload{
		RequesterID:    "owner-1",
		ServiceID:      svc.ID,
		TemplateID:     uuid.NewString(),
		InstanceSizeID: uuid.NewString(),
		Namespace:      "prod-shop",
		Reason:         "create before delete",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	eventID := "ev-" + uuid.NewString()
	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID(svc.ID).
		SetPayload(payload).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("owner-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create domain event: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID("ticket-" + uuid.NewString()).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusPENDING).
		SetRequester("owner-1").
		SetReason("create before delete").
		Save(t.Context()); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+sys.ID+"/services/"+svc.ID+"?confirm=true",
		"",
		"owner-1",
		[]string{"service:delete"},
	)
	srv.DeleteService(c, sys.ID, svc.ID, generated.DeleteServiceParams{Confirm: true})
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "SERVICE_HAS_ACTIVE_REQUESTS")
}

func newSystemBehaviorTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	client, pool := testutil.OpenEntPostgresWithPool(t, "system_handler_behavior")
	return NewServer(ServerDeps{EntClient: client, Pool: pool}), client
}

func newAuthedGinContext(
	t *testing.T,
	method string,
	target string,
	body string,
	userID string,
	permissions []string,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var req *http.Request
	if strings.TrimSpace(body) == "" {
		req = httptest.NewRequest(method, target, http.NoBody)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}

	req = req.WithContext(middleware.SetUserContext(req.Context(), userID, userID, nil))
	c.Request = req
	c.Set("permissions", permissions)
	return c, w
}

func mustCreateSystem(t *testing.T, client *ent.Client, id, name, createdBy string) *ent.System {
	t.Helper()
	obj, err := client.System.Create().
		SetID(id).
		SetName(name).
		SetCreatedBy(createdBy).
		SetDescription("init").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	return obj
}

func mustCreateService(t *testing.T, client *ent.Client, id, name, systemID, description string) *ent.Service {
	t.Helper()
	obj, err := client.Service.Create().
		SetID(id).
		SetName(name).
		SetDescription(description).
		SetSystemID(systemID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return obj
}

func mustCreateVMForService(t *testing.T, client *ent.Client, id, name, serviceID string) *ent.VM {
	t.Helper()
	obj, err := client.VM.Create().
		SetID(id).
		SetName(name).
		SetInstance("01").
		SetNamespace("ns-test").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("owner-1").
		SetServiceID(serviceID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	return obj
}

func mustCreateSystemBinding(t *testing.T, client *ent.Client, userID, systemID, role string) {
	t.Helper()
	_, err := client.ResourceRoleBinding.Create().
		SetID(uuid.NewString()).
		SetUserID(userID).
		SetResourceType("system").
		SetResourceID(systemID).
		SetRole(rrb.Role(role)).
		SetCreatedBy("test-seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create resource role binding: %v", err)
	}
}

func mustCreateUserForSystemSearch(t *testing.T, client *ent.Client, id, email, displayName string) {
	t.Helper()
	_, err := client.User.Create().
		SetID(id).
		SetUsername(email).
		SetEmail(email).
		SetDisplayName(displayName).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
}
