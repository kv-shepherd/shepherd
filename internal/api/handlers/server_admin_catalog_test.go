package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestAdminTemplateCRUD(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"ubuntu-base","display_name":"Ubuntu Base","description":"base image","catalog_scope":"prod","os_family":"linux","os_version":"22.04","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminTemplate(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created generated.Template
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if created.Id == "" || created.Name != "ubuntu-base" {
		t.Fatalf("unexpected created template: %+v", created)
	}
	if created.CatalogScope != "prod" {
		t.Fatalf("catalog_scope = %q, want %q", created.CatalogScope, "prod")
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/templates/"+created.Id,
		`{"display_name":"Ubuntu Base v2","catalog_scope":"all","enabled":false}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAdminTemplate(updateCtx, created.Id)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	var updated generated.Template
	mustDecodeJSON(t, updateW.Body.Bytes(), &updated)
	if updated.DisplayName != "Ubuntu Base v2" {
		t.Fatalf("display_name = %q, want %q", updated.DisplayName, "Ubuntu Base v2")
	}
	if updated.CatalogScope != "all" {
		t.Fatalf("catalog_scope = %q, want %q", updated.CatalogScope, "all")
	}
	if updated.Enabled {
		t.Fatal("expected template enabled=false after update")
	}

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/templates?page=1&per_page=20",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAdminTemplates(listCtx, generated.ListAdminTemplatesParams{})
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/templates/"+created.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAdminTemplate(deleteCtx, created.Id)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}

	if _, err := client.Template.Get(t.Context(), created.Id); !ent.IsNotFound(err) {
		t.Fatalf("expected template deleted, err=%v", err)
	}
}

func TestAdminInstanceSizeCRUD(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/instance-sizes",
		`{"name":"m4.large","display_name":"M4 Large","catalog_scope":"test","cpu_cores":4,"memory_gi":8,"disk_gb":80,"cpu_request":2,"memory_request_gi":6,"sort_order":20,"dedicated_cpu":false,"enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminInstanceSize(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created generated.InstanceSize
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if created.Id == "" || created.Name != "m4.large" {
		t.Fatalf("unexpected created instance size: %+v", created)
	}
	if created.CatalogScope != "test" {
		t.Fatalf("catalog_scope = %q, want %q", created.CatalogScope, "test")
	}
	if created.CpuRequest != 2 {
		t.Fatalf("cpu_request = %v, want 2", created.CpuRequest)
	}
	if created.MemoryRequestGi != 6 {
		t.Fatalf("memory_request_gi = %v, want 6", created.MemoryRequestGi)
	}
	if created.SortOrder != 20 {
		t.Fatalf("sort_order = %d, want 20", created.SortOrder)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/instance-sizes/"+created.Id,
		`{"display_name":"M4 Large Updated","catalog_scope":"all","requires_gpu":true,"enabled":false}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAdminInstanceSize(updateCtx, created.Id)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	var updated generated.InstanceSize
	mustDecodeJSON(t, updateW.Body.Bytes(), &updated)
	if updated.DisplayName != "M4 Large Updated" {
		t.Fatalf("display_name = %q, want %q", updated.DisplayName, "M4 Large Updated")
	}
	if updated.CatalogScope != "all" {
		t.Fatalf("catalog_scope = %q, want %q", updated.CatalogScope, "all")
	}
	if !updated.RequiresGpu {
		t.Fatal("expected requires_gpu=true after update")
	}
	if updated.CpuRequest != 2 {
		t.Fatalf("cpu_request = %v, want 2 after unchanged update", updated.CpuRequest)
	}

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/instance-sizes",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAdminInstanceSizes(listCtx)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/instance-sizes/"+created.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAdminInstanceSize(deleteCtx, created.Id)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}

	if _, err := client.InstanceSize.Get(t.Context(), created.Id); !ent.IsNotFound(err) {
		t.Fatalf("expected instance size deleted, err=%v", err)
	}
}

func TestAdminInstanceSize_DerivesHugepagesHintsFromSpecOverrides(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/instance-sizes",
		`{
			"name":"m4-hugepages",
			"display_name":"M4 Hugepages",
			"catalog_scope":"prod",
			"cpu_cores":4,
			"memory_gi":8,
			"enabled":true,
			"spec_overrides":{
				"spec":{
					"template":{
						"spec":{
							"domain":{
								"memory":{
									"hugepages":{
										"pageSize":"2Mi"
									}
								}
							}
						}
					}
				}
			}
		}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminInstanceSize(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created generated.InstanceSize
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if !created.RequiresHugepages {
		t.Fatal("requires_hugepages = false, want true")
	}
	if created.HugepagesSize != "2Mi" {
		t.Fatalf("hugepages_size = %q, want 2Mi", created.HugepagesSize)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/instance-sizes/"+created.Id,
		`{
			"spec_overrides":{
				"spec":{
					"template":{
						"spec":{
							"domain":{
								"cpu":{
									"cores":4
								}
							}
						}
					}
				}
			}
		}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAdminInstanceSize(updateCtx, created.Id)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	var updated generated.InstanceSize
	mustDecodeJSON(t, updateW.Body.Bytes(), &updated)
	if updated.RequiresHugepages {
		t.Fatal("requires_hugepages = true, want false after hugepages removed from spec")
	}
	if updated.HugepagesSize != "" {
		t.Fatalf("hugepages_size = %q, want empty after hugepages removed from spec", updated.HugepagesSize)
	}
}

func TestDeleteAdminTemplate_RejectsActiveCreateRequests(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)
	ctx := t.Context()

	tpl, err := client.Template.Create().
		SetID("tpl-active-req").
		SetName("ubuntu-active-req").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	payload, err := json.Marshal(domain.VMCreationPayload{
		RequesterID:    "user-a",
		ServiceID:      "svc-a",
		TemplateID:     tpl.ID,
		InstanceSizeID: "size-a",
		Namespace:      "prod-a",
		Reason:         "pending request",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := client.DomainEvent.Create().
		SetID("ev-template-active-req").
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
		SetID("ticket-template-active-req").
		SetEventID("ev-template-active-req").
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusAPPROVED).
		SetRequester("user-a").
		SetReason("pending request").
		Save(ctx); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/templates/"+tpl.ID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAdminTemplate(deleteCtx, tpl.ID)
	if deleteW.Code != http.StatusConflict {
		t.Fatalf("delete status = %d, want %d, body=%s", deleteW.Code, http.StatusConflict, deleteW.Body.String())
	}
	assertErrorCode(t, deleteW.Body.Bytes(), "TEMPLATE_HAS_ACTIVE_REQUESTS")
}

func TestDeleteAdminInstanceSize_RejectsActiveCreateRequests(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)
	ctx := t.Context()

	size, err := client.InstanceSize.Create().
		SetID("size-active-req").
		SetName("m4-active-req").
		SetCPUCores(4).
		SetMemoryGi(8).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	payload, err := json.Marshal(domain.VMCreationPayload{
		RequesterID:    "user-a",
		ServiceID:      "svc-a",
		TemplateID:     "tpl-a",
		InstanceSizeID: size.ID,
		Namespace:      "prod-a",
		Reason:         "pending request",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := client.DomainEvent.Create().
		SetID("ev-size-active-req").
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
		SetID("ticket-size-active-req").
		SetEventID("ev-size-active-req").
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusEXECUTING).
		SetRequester("user-a").
		SetReason("pending request").
		Save(ctx); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/instance-sizes/"+size.ID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAdminInstanceSize(deleteCtx, size.ID)
	if deleteW.Code != http.StatusConflict {
		t.Fatalf("delete status = %d, want %d, body=%s", deleteW.Code, http.StatusConflict, deleteW.Body.String())
	}
	assertErrorCode(t, deleteW.Body.Bytes(), "INSTANCE_SIZE_HAS_ACTIVE_REQUESTS")
}

func TestAdminInstanceSizeUpdate_EmptyDvAccessModesClearsExplicitRootVolumeMode(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	created, err := client.InstanceSize.Create().
		SetID(uuid.NewString()).
		SetName("m4.block-rwx").
		SetCPUCores(4).
		SetMemoryGi(8).
		SetDvAccessModes([]string{"ReadWriteMany"}).
		SetDvVolumeMode("Block").
		SetCreatedBy("test").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/instance-sizes/"+created.ID,
		`{"dv_access_modes":[]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAdminInstanceSize(updateCtx, created.ID)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	stored, err := client.InstanceSize.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("load instance size: %v", err)
	}
	if len(stored.DvAccessModes) != 0 {
		t.Fatalf("dv_access_modes = %#v, want cleared", stored.DvAccessModes)
	}
	if stored.DvVolumeMode != "" {
		t.Fatalf("dv_volume_mode = %q, want cleared", stored.DvVolumeMode)
	}
}

func TestAdminTemplateCreate_StoresCanonicalSourceType(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"ubuntu-base","source_type":"cdi_pvc_clone","pvc_name":"golden-ubuntu","pvc_namespace":"golden-images","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminTemplate(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created generated.Template
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if got, want := string(created.SourceType), "cdi_pvc_clone"; got != want {
		t.Fatalf("source_type = %q, want %q", got, want)
	}

	stored, err := client.Template.Get(t.Context(), created.Id)
	if err != nil {
		t.Fatalf("load created template: %v", err)
	}
	if got, want := stored.SourceType, "cdi_pvc_clone"; got != want {
		t.Fatalf("stored source_type = %q, want %q", got, want)
	}
}

func TestAdminTemplateCreate_RejectsLegacySourceTypeAlias(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"ubuntu-base","source_type":"pvc","pvc_name":"golden-ubuntu","pvc_namespace":"golden-images","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminTemplate(createCtx)
	if createW.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusBadRequest, createW.Body.String())
	}
}

func TestAdminTemplateCreate_RejectsContainerDiskForProdCatalog(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminCatalogTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/templates",
		`{"name":"fedora-ephemeral","source_type":"containerdisk","image_url":"quay.io/containerdisks/fedora:40","catalog_scope":"prod","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAdminTemplate(createCtx)
	if createW.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d, body=%s", createW.Code, http.StatusBadRequest, createW.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, createW.Body.Bytes(), &resp)
	if got, want := resp.Code, "INVALID_TEMPLATE_SOURCE_SCOPE"; got != want {
		t.Fatalf("error code = %q, want %q", got, want)
	}
}

func TestAdminTemplateUpdate_RejectsPromotingContainerDiskToAllCatalog(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)

	templateID := "tpl-" + uuid.NewString()
	_, err := client.Template.Create().
		SetID(templateID).
		SetName("fedora-ephemeral").
		SetSourceType("containerdisk").
		SetImageURL("quay.io/containerdisks/fedora:40").
		SetCatalogScope("test").
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/templates/"+templateID,
		`{"catalog_scope":"all"}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAdminTemplate(updateCtx, templateID)
	if updateW.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusBadRequest, updateW.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, updateW.Body.Bytes(), &resp)
	if got, want := resp.Code, "INVALID_TEMPLATE_SOURCE_SCOPE"; got != want {
		t.Fatalf("error code = %q, want %q", got, want)
	}
}

func TestUpdateRole_RollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	client := testutil.OpenEntPostgres(t, "admin_catalog_update_role_revoke_fail")
	srv := NewServer(ServerDeps{
		EntClient:    client,
		AuthSessions: &service.AuthSessionManager{},
	})

	roleRow, err := client.Role.Create().
		SetID("role-update-revoke-fail").
		SetName("Operator").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/roles/"+roleRow.ID,
		`{"enabled":false}`,
		"admin-1",
		[]string{"rbac:manage"},
	)
	srv.UpdateRole(updateCtx, roleRow.ID)
	if updateW.Code != http.StatusInternalServerError {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusInternalServerError, updateW.Body.String())
	}

	reloaded, err := client.Role.Get(t.Context(), roleRow.ID)
	if err != nil {
		t.Fatalf("reload role: %v", err)
	}
	if !reloaded.Enabled {
		t.Fatal("expected role to remain enabled after failed session revocation")
	}
}

func TestDeleteRoleRejectsExternalCohortMappingReference(t *testing.T) {
	t.Parallel()

	srv, client := newAdminCatalogTestServer(t)
	ctx := t.Context()

	roleRow, err := client.Role.Create().
		SetID("role-delete-cohort-mapping-in-use").
		SetName("DeleteCohortMappingInUse").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	mappingRow, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-role-in-use").
		SetProviderID("provider-delete-role-in-use").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleRow.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort mapping: %v", err)
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/roles/"+roleRow.ID,
		"",
		"admin-1",
		[]string{"rbac:manage"},
	)
	srv.DeleteRole(deleteCtx, roleRow.ID)
	if deleteW.Code != http.StatusConflict {
		t.Fatalf("delete role status = %d, want %d, body=%s", deleteW.Code, http.StatusConflict, deleteW.Body.String())
	}
	var resp generated.Error
	mustDecodeJSON(t, deleteW.Body.Bytes(), &resp)
	if resp.Code != "ROLE_IN_USE" {
		t.Fatalf("delete role error code = %q, want ROLE_IN_USE", resp.Code)
	}
	if _, err := client.Role.Get(ctx, roleRow.ID); err != nil {
		t.Fatalf("role should remain: %v", err)
	}
	if _, err := client.ExternalCohortMapping.Get(ctx, mappingRow.ID); err != nil {
		t.Fatalf("mapping should remain: %v", err)
	}
}

func newAdminCatalogTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	client := testutil.OpenEntPostgres(t, "admin_catalog")
	return NewServer(ServerDeps{
		EntClient:     client,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	}), client
}

func mustDecodeJSON(t *testing.T, payload []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(payload, out); err != nil {
		t.Fatalf("decode json: %v; payload=%s", err, string(payload))
	}
}
