package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/schema"
)

// TestGetDynamicSchema does not require a database — the handler reads only
// from in-process embedded schema files. We construct a *Server{} with zero
// values deliberately; the handler does not access any Server fields.
func TestGetDynamicSchema_Instancesize_Returns200(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	srv := &Server{} // no DB needed for this handler
	c, w := newSchemaGinContext(t, "instancesize")

	srv.GetDynamicSchema(c, generated.Instancesize)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.DynamicSchemaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}

	// schema must be a non-empty map.
	if len(resp.Schema) == 0 {
		t.Error("response schema is empty, want non-empty map")
	}

	// quick_fields must be present (may be empty list but not nil).
	if resp.Mask.QuickFields == nil {
		t.Error("mask.quick_fields is nil, want array")
	}
	if len(resp.Mask.QuickFields) == 0 {
		t.Error("expected at least one quick_field for instancesize")
	}

	// schema_version must be set.
	if resp.SchemaVersion == "" {
		t.Error("schema_version is empty, want semver string")
	}

	expectedVersion, ok := schema.SchemaVersionFor("instancesize")
	if !ok {
		t.Fatal("schema.SchemaVersionFor(instancesize) = !ok, want true")
	}
	if resp.SchemaVersion != expectedVersion {
		t.Errorf("schema_version = %q, want %q", resp.SchemaVersion, expectedVersion)
	}

	// source must be "embedded" (current implementation serves from embedded baseline).
	if resp.Source != generated.Embedded {
		t.Errorf("source = %q, want %q", resp.Source, generated.Embedded)
	}

	// degraded must be false (embedded IS the source of truth now, not a fallback).
	if resp.Degraded {
		t.Error("degraded = true, want false for embedded schema")
	}
}

func TestGetDynamicSchema_Template_Returns400(t *testing.T) {
	// template is excluded from the dynamic schema endpoint.
	// Templates use static fields only (cloud_init YAML textarea, master-flow Step 3).
	t.Parallel()
	gin.SetMode(gin.TestMode)

	srv := &Server{}
	c, w := newSchemaGinContext(t, "template")

	srv.GetDynamicSchema(c, generated.GetDynamicSchemaParamsEntityType("template"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (template has no dynamic schema), body=%s",
			w.Code, http.StatusBadRequest, w.Body.String())
	}

	var errResp generated.Error
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != "UNSUPPORTED_ENTITY_TYPE" {
		t.Errorf("error code = %q, want %q", errResp.Code, "UNSUPPORTED_ENTITY_TYPE")
	}
}

func TestGetDynamicSchema_Cluster_Returns400(t *testing.T) {
	// cluster is not a supported entity_type (no schema defined, ADR-0023 phase 2).
	t.Parallel()
	gin.SetMode(gin.TestMode)

	srv := &Server{}
	c, w := newSchemaGinContext(t, "cluster")

	// Cast string directly to GetDynamicSchemaParamsEntityType to simulate
	// a raw request; oapi-codegen enum will not include cluster.
	srv.GetDynamicSchema(c, generated.GetDynamicSchemaParamsEntityType("cluster"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (cluster has no real schema), body=%s",
			w.Code, http.StatusBadRequest, w.Body.String())
	}

	var errResp generated.Error
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != "UNSUPPORTED_ENTITY_TYPE" {
		t.Errorf("error code = %q, want %q", errResp.Code, "UNSUPPORTED_ENTITY_TYPE")
	}
}

// TestGetDynamicSchema_InstancesizeMask_ContainsHugepages verifies that the
// instancesize mask quick_fields contain a high-frequency path (integration check).
func TestGetDynamicSchema_InstancesizeMask_ContainsHugepages(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	srv := &Server{}
	c, w := newSchemaGinContext(t, "instancesize")
	srv.GetDynamicSchema(c, generated.Instancesize)

	var resp generated.DynamicSchemaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	hugepagesPath := "spec.template.spec.domain.memory.hugepages.pageSize"
	found := false
	for _, f := range resp.Mask.QuickFields {
		if f.Path == hugepagesPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("quick_fields does not contain path %q; got %+v", hugepagesPath, resp.Mask.QuickFields)
	}
}

func TestGetDynamicSchema_InstancesizeMask_RetainsMetadataKeys(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	srv := &Server{}
	c, w := newSchemaGinContext(t, "instancesize")
	srv.GetDynamicSchema(c, generated.Instancesize)

	var resp generated.DynamicSchemaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	targetPath := "spec.template.spec.domain.memory.hugepages.pageSize"
	var target *generated.MaskField
	for i := range resp.Mask.QuickFields {
		if resp.Mask.QuickFields[i].Path == targetPath {
			target = &resp.Mask.QuickFields[i]
			break
		}
	}
	if target == nil {
		t.Fatalf("quick_fields does not contain path %q", targetPath)
	}
	if target.DisplayNameKey == "" {
		t.Errorf("display_name_key is empty for %q", targetPath)
	}
	if target.HelpKey == "" {
		t.Errorf("help_key is empty for %q", targetPath)
	}
	if target.PlaceholderKey == "" {
		t.Errorf("placeholder_key is empty for %q", targetPath)
	}
}

func TestGetDynamicSchema_InstancesizeMask_ContainsProfessionalFields(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	srv := &Server{}
	c, w := newSchemaGinContext(t, "instancesize")
	srv.GetDynamicSchema(c, generated.Instancesize)

	var resp generated.DynamicSchemaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	targetPath := "spec.template.spec.domain.features.hyperv.relaxed.enabled"
	found := false
	for _, f := range resp.Mask.ProfessionalFields {
		if f.Path == targetPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("professional_fields does not contain path %q; got %+v", targetPath, resp.Mask.ProfessionalFields)
	}
}

// newSchemaGinContext builds a minimal gin.Context for GET /schemas/{entityType}.
// /schemas/ is a public endpoint (OpenAPI security:[], router.go publicPrefixes).
// No authentication headers are required.
func newSchemaGinContext(t *testing.T, entityType string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/schemas/"+entityType, http.NoBody)
	c.Request = req
	return c, w
}
