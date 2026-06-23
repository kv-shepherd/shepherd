package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/auditlog"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	"kv-shepherd.io/shepherd/ent/pendingadoption"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestListPendingAdoptions_FiltersAndPaginates(t *testing.T) {
	srv, client := newAdminAdoptionTestServer(t)
	ctx := t.Context()
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-list-prod",
		clusterID:    "cluster-prod",
		namespace:    "team-a",
		resourceName: "vm-prod-01",
		status:       pendingadoption.StatusPENDING,
		discoveredBy: "system:vm-adoption-discovery",
	})
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-list-rejected",
		clusterID:    "cluster-prod",
		namespace:    "team-a",
		resourceName: "vm-prod-ignored",
		status:       pendingadoption.StatusREJECTED,
		discoveredBy: "admin-1",
	})
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-list-test",
		clusterID:    "cluster-test",
		namespace:    "team-b",
		resourceName: "vm-test-01",
		status:       pendingadoption.StatusPENDING,
		discoveredBy: "scanner-test",
	})

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/pending-adoptions?status=PENDING&cluster_id=cluster-prod&search=prod",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListPendingAdoptions(c, generated.ListPendingAdoptionsParams{
		Status:    generated.PendingAdoptionStatusPENDING,
		ClusterId: "cluster-prod",
		Search:    "prod",
		Page:      1,
		PerPage:   20,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp generated.PendingAdoptionList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	if resp.Items[0].Id != "pa-list-prod" {
		t.Fatalf("item id = %q, want pa-list-prod", resp.Items[0].Id)
	}
	if resp.Pagination.Total != 1 || resp.Pagination.Page != 1 || resp.Pagination.PerPage != 20 {
		t.Fatalf("pagination = %+v, want total=1 page=1 per_page=20", resp.Pagination)
	}

	exists, err := client.PendingAdoption.Query().
		Where(pendingadoption.IDEQ("pa-list-prod")).
		Exist(ctx)
	require.NoError(t, err)
	if !exists {
		t.Fatal("list should not mutate pending adoption rows")
	}
}

func TestListPendingAdoptions_RejectsInvalidStatus(t *testing.T) {
	srv, _ := newAdminAdoptionTestServer(t)

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/pending-adoptions?status=BOGUS",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListPendingAdoptions(c, generated.ListPendingAdoptionsParams{
		Status: generated.PendingAdoptionStatus("BOGUS"),
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestListPendingAdoptions_RejectsInvalidResourceType(t *testing.T) {
	srv, _ := newAdminAdoptionTestServer(t)

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/pending-adoptions?resource_type=Service",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListPendingAdoptions(c, generated.ListPendingAdoptionsParams{
		ResourceType: generated.PendingAdoptionResourceType("Service"),
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}

func TestRejectPendingAdoption_RejectsPendingRowAndAudits(t *testing.T) {
	srv, client := newAdminAdoptionTestServer(t)
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-reject",
		clusterID:    "cluster-prod",
		namespace:    "team-a",
		resourceName: "vm-prod-01",
		status:       pendingadoption.StatusPENDING,
		discoveredBy: "system:vm-adoption-discovery",
	})

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/pending-adoptions/pa-reject/reject",
		`{"reason":"manual cleanup completed"}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.RejectPendingAdoption(c, "pa-reject")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp generated.PendingAdoption
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	if resp.Status != generated.PendingAdoptionStatusREJECTED {
		t.Fatalf("response status = %s, want REJECTED", resp.Status)
	}

	row, err := client.PendingAdoption.Get(t.Context(), "pa-reject")
	require.NoError(t, err)
	if row.Status != pendingadoption.StatusREJECTED {
		t.Fatalf("db status = %s, want REJECTED", row.Status)
	}

	auditRow, err := client.AuditLog.Query().
		Where(
			auditlog.ActionEQ("adoption.rejected"),
			auditlog.ResourceTypeEQ("pending_adoption"),
			auditlog.ResourceIDEQ("pa-reject"),
		).
		Only(t.Context())
	require.NoError(t, err)
	if auditRow.Actor != "admin-1" {
		t.Fatalf("audit actor = %q, want admin-1", auditRow.Actor)
	}
	if auditRow.Details["reason"] != "manual cleanup completed" {
		t.Fatalf("audit reason = %#v, want manual cleanup completed", auditRow.Details["reason"])
	}
}

func TestRejectPendingAdoption_RejectsOnlyPendingRows(t *testing.T) {
	srv, client := newAdminAdoptionTestServer(t)
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-already-rejected",
		clusterID:    "cluster-prod",
		namespace:    "team-a",
		resourceName: "vm-prod-01",
		status:       pendingadoption.StatusREJECTED,
	})

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/pending-adoptions/pa-already-rejected/reject",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.RejectPendingAdoption(c, "pa-already-rejected")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "PENDING_ADOPTION_NOT_PENDING")
}

func TestRejectPendingAdoption_DoesNotOverwriteConcurrentDecision(t *testing.T) {
	srv, client := newAdminAdoptionTestServer(t)
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-reject-race",
		clusterID:    "cluster-prod",
		namespace:    "team-a",
		resourceName: "vm-prod-01",
		status:       pendingadoption.StatusPENDING,
	})
	injectPendingAdoptionStatusBeforeNextUpdate(t, client, "pa-reject-race", pendingadoption.StatusADOPTED)

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/pending-adoptions/pa-reject-race/reject",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.RejectPendingAdoption(c, "pa-reject-race")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "PENDING_ADOPTION_NOT_PENDING")
	row, err := client.PendingAdoption.Get(t.Context(), "pa-reject-race")
	require.NoError(t, err)
	if row.Status != pendingadoption.StatusADOPTED {
		t.Fatalf("pending adoption status = %s, want ADOPTED", row.Status)
	}
	auditExists, err := client.AuditLog.Query().
		Where(
			auditlog.ActionEQ("adoption.rejected"),
			auditlog.ResourceIDEQ("pa-reject-race"),
		).
		Exist(t.Context())
	require.NoError(t, err)
	if auditExists {
		t.Fatal("stale reject must not write a rejection audit log")
	}
}

func TestAdoptPendingAdoption_AdoptsLiveVMAndAudits(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            "vm-prod-01",
		Namespace:       "team-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-adopt-1",
		Spec: domain.VMSpec{Labels: map[string]string{
			domain.ShepherdServiceIDLabel:  "svc-adoption-admin",
			domain.ShepherdTemplateIDLabel: "tpl-adopt",
			domain.ShepherdEventIDLabel:    "evt-pa-adopt",
		}},
	}})
	srv, client := newAdminAdoptionTestServerWithVMService(t, service.NewVMService(mock))
	mustCreateAdoptionService(t, client, "svc-adoption-admin")
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-adopt",
		clusterID:    "cluster-prod",
		namespace:    "team-a",
		resourceName: "vm-prod-01",
		status:       pendingadoption.StatusPENDING,
		discoveredBy: "system:vm-adoption-discovery",
	})

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/pending-adoptions/pa-adopt/adopt",
		`{"reason":"recover inventory"}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.AdoptPendingAdoption(c, "pa-adopt")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp generated.PendingAdoptionAdoptResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	if resp.PendingAdoption.Status != generated.PendingAdoptionStatusADOPTED {
		t.Fatalf("response adoption status = %s, want ADOPTED", resp.PendingAdoption.Status)
	}
	if resp.VmId == "" || resp.VmName != "vm-prod-01" {
		t.Fatalf("adopt response vm = (%q,%q), want non-empty id and vm-prod-01 name", resp.VmId, resp.VmName)
	}

	vmRow, err := client.VM.Query().
		Where(entvm.NamespaceEQ("team-a"), entvm.NameEQ("vm-prod-01")).
		Only(t.Context())
	require.NoError(t, err)
	if vmRow.ID != resp.VmId {
		t.Fatalf("vm id = %q, want response id %q", vmRow.ID, resp.VmId)
	}
	if vmRow.Status != entvm.StatusRUNNING {
		t.Fatalf("vm status = %s, want RUNNING", vmRow.Status)
	}
	if vmRow.Instance != "01" {
		t.Fatalf("vm instance = %q, want 01", vmRow.Instance)
	}
	if vmRow.ClusterID != "cluster-prod" || vmRow.CreatedBy != "admin-1" {
		t.Fatalf("vm placement/actor = (%q,%q), want cluster-prod/admin-1", vmRow.ClusterID, vmRow.CreatedBy)
	}
	if vmRow.LastK8sRv == nil || *vmRow.LastK8sRv != "rv-adopt-1" {
		t.Fatalf("vm last_k8s_rv = %#v, want rv-adopt-1", vmRow.LastK8sRv)
	}
	svc, err := vmRow.QueryService().Only(t.Context())
	require.NoError(t, err)
	if svc.ID != "svc-adoption-admin" {
		t.Fatalf("vm service id = %q, want svc-adoption-admin", svc.ID)
	}

	adoptionRow, err := client.PendingAdoption.Get(t.Context(), "pa-adopt")
	require.NoError(t, err)
	if adoptionRow.Status != pendingadoption.StatusADOPTED {
		t.Fatalf("pending adoption status = %s, want ADOPTED", adoptionRow.Status)
	}
	if adoptionRow.Labels[domain.ShepherdTemplateIDLabel] != "tpl-adopt" {
		t.Fatalf("pending adoption template label = %q, want live tpl-adopt", adoptionRow.Labels[domain.ShepherdTemplateIDLabel])
	}

	auditRow, err := client.AuditLog.Query().
		Where(
			auditlog.ActionEQ("adoption.adopted"),
			auditlog.ResourceTypeEQ("pending_adoption"),
			auditlog.ResourceIDEQ("pa-adopt"),
		).
		Only(t.Context())
	require.NoError(t, err)
	if auditRow.Actor != "admin-1" {
		t.Fatalf("audit actor = %q, want admin-1", auditRow.Actor)
	}
	if auditRow.Details["reason"] != "recover inventory" || auditRow.Details["vm_id"] != resp.VmId {
		t.Fatalf("audit details = %#v, want reason and vm_id", auditRow.Details)
	}
}

func TestAdoptPendingAdoption_RejectsExistingVMRow(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:      "vm-prod-01",
		Namespace: "team-a",
		Status:    domain.VMStatusRunning,
		Spec: domain.VMSpec{Labels: map[string]string{
			domain.ShepherdServiceIDLabel: "svc-adoption-admin",
		}},
	}})
	srv, client := newAdminAdoptionTestServerWithVMService(t, service.NewVMService(mock))
	mustCreateAdoptionService(t, client, "svc-adoption-admin")
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-adopt-existing",
		clusterID:    "cluster-prod",
		namespace:    "team-a",
		resourceName: "vm-prod-01",
		status:       pendingadoption.StatusPENDING,
	})
	_, err := client.VM.Create().
		SetID("vm-existing").
		SetName("vm-prod-01").
		SetInstance("01").
		SetNamespace("team-a").
		SetClusterID("cluster-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("seed").
		SetServiceID("svc-adoption-admin").
		Save(t.Context())
	require.NoError(t, err)

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/pending-adoptions/pa-adopt-existing/adopt",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.AdoptPendingAdoption(c, "pa-adopt-existing")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "PENDING_ADOPTION_VM_EXISTS")
	row, err := client.PendingAdoption.Get(t.Context(), "pa-adopt-existing")
	require.NoError(t, err)
	if row.Status != pendingadoption.StatusPENDING {
		t.Fatalf("pending adoption status = %s, want PENDING", row.Status)
	}
}

func TestAdoptPendingAdoption_DoesNotCommitVMWhenDecisionBecomesStale(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            "vm-prod-01",
		Namespace:       "team-a",
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-adopt-race",
		Spec: domain.VMSpec{Labels: map[string]string{
			domain.ShepherdServiceIDLabel: "svc-adoption-admin",
		}},
	}})
	srv, client := newAdminAdoptionTestServerWithVMService(t, service.NewVMService(mock))
	mustCreateAdoptionService(t, client, "svc-adoption-admin")
	mustCreatePendingAdoption(t, client, pendingAdoptionSeed{
		id:           "pa-adopt-race",
		clusterID:    "cluster-prod",
		namespace:    "team-a",
		resourceName: "vm-prod-01",
		status:       pendingadoption.StatusPENDING,
	})
	injectPendingAdoptionStatusBeforeNextUpdate(t, client, "pa-adopt-race", pendingadoption.StatusREJECTED)

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/pending-adoptions/pa-adopt-race/adopt",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.AdoptPendingAdoption(c, "pa-adopt-race")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "PENDING_ADOPTION_NOT_PENDING")
	row, err := client.PendingAdoption.Get(t.Context(), "pa-adopt-race")
	require.NoError(t, err)
	if row.Status != pendingadoption.StatusREJECTED {
		t.Fatalf("pending adoption status = %s, want REJECTED", row.Status)
	}
	vmExists, err := client.VM.Query().
		Where(entvm.NamespaceEQ("team-a"), entvm.NameEQ("vm-prod-01")).
		Exist(t.Context())
	require.NoError(t, err)
	if vmExists {
		t.Fatal("stale adoption must roll back the created VM row")
	}
	auditExists, err := client.AuditLog.Query().
		Where(
			auditlog.ActionEQ("adoption.adopted"),
			auditlog.ResourceIDEQ("pa-adopt-race"),
		).
		Exist(t.Context())
	require.NoError(t, err)
	if auditExists {
		t.Fatal("stale adoption must not write an adoption audit log")
	}
}

func TestPendingAdoptionAdminHandlers_RequirePlatformAdmin(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerDeps{})
	listCtx, listW := newAuthedGinContext(t, http.MethodGet, "/admin/pending-adoptions", "", "user-a", []string{"vm:read"})
	srv.ListPendingAdoptions(listCtx, generated.ListPendingAdoptionsParams{})
	if listW.Code != http.StatusForbidden {
		t.Fatalf("list status = %d, want %d body=%s", listW.Code, http.StatusForbidden, listW.Body.String())
	}
	assertErrorCode(t, listW.Body.Bytes(), "FORBIDDEN")

	rejectCtx, rejectW := newAuthedGinContext(t, http.MethodPost, "/admin/pending-adoptions/pa-1/reject", "", "user-a", []string{"vm:operate"})
	srv.RejectPendingAdoption(rejectCtx, "pa-1")
	if rejectW.Code != http.StatusForbidden {
		t.Fatalf("reject status = %d, want %d body=%s", rejectW.Code, http.StatusForbidden, rejectW.Body.String())
	}
	assertErrorCode(t, rejectW.Body.Bytes(), "FORBIDDEN")

	adoptCtx, adoptW := newAuthedGinContext(t, http.MethodPost, "/admin/pending-adoptions/pa-1/adopt", "", "user-a", []string{"vm:operate"})
	srv.AdoptPendingAdoption(adoptCtx, "pa-1")
	if adoptW.Code != http.StatusForbidden {
		t.Fatalf("adopt status = %d, want %d body=%s", adoptW.Code, http.StatusForbidden, adoptW.Body.String())
	}
	assertErrorCode(t, adoptW.Body.Bytes(), "FORBIDDEN")
}

type pendingAdoptionSeed struct {
	id           string
	clusterID    string
	namespace    string
	resourceName string
	status       pendingadoption.Status
	discoveredBy string
}

func newAdminAdoptionTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	return newAdminAdoptionTestServerWithVMService(t, nil)
}

func newAdminAdoptionTestServerWithVMService(t *testing.T, vmService *service.VMService) (*Server, *ent.Client) {
	t.Helper()
	client := testutil.OpenEntPostgres(t, "admin_adoption_"+uuid.NewString()[:8])
	return NewServer(ServerDeps{
		EntClient: client,
		Audit:     audit.NewLogger(client),
		VMService: vmService,
	}), client
}

func mustCreatePendingAdoption(t *testing.T, client *ent.Client, seed pendingAdoptionSeed) *ent.PendingAdoption {
	t.Helper()
	create := client.PendingAdoption.Create().
		SetID(seed.id).
		SetClusterID(seed.clusterID).
		SetNamespace(seed.namespace).
		SetResourceName(seed.resourceName).
		SetResourceType("VirtualMachine").
		SetStatus(seed.status).
		SetLabels(map[string]string{
			domain.ShepherdServiceIDLabel: "svc-adoption-admin",
			domain.ShepherdEventIDLabel:   "evt-" + seed.id,
		})
	if seed.discoveredBy != "" {
		create.SetDiscoveredBy(seed.discoveredBy)
	}
	row, err := create.Save(t.Context())
	require.NoError(t, err)
	return row
}

func mustCreateAdoptionService(t *testing.T, client *ent.Client, serviceID string) {
	t.Helper()
	systemID := "sys-" + uuid.NewString()[:8]
	_, err := client.System.Create().
		SetID(systemID).
		SetName("sys" + uuid.NewString()[:6]).
		SetCreatedBy("seed").
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Service.Create().
		SetID(serviceID).
		SetName("svc" + uuid.NewString()[:6]).
		SetSystemID(systemID).
		Save(t.Context())
	require.NoError(t, err)
}

func injectPendingAdoptionStatusBeforeNextUpdate(t *testing.T, client *ent.Client, id string, status pendingadoption.Status) {
	t.Helper()
	injected := false
	client.PendingAdoption.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			mutation, ok := m.(*ent.PendingAdoptionMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}
			mutationID, ok := mutation.ID()
			if !injected && ok && mutationID == id {
				injected = true
				if _, err := client.PendingAdoption.UpdateOneID(id).
					SetStatus(status).
					Save(ctx); err != nil {
					return nil, err
				}
			}
			return next.Mutate(ctx, m)
		})
	}, ent.OpUpdateOne))
}
