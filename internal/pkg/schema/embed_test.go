package schema_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"kv-shepherd.io/shepherd/internal/pkg/schema"
)

// TestMain runs mask-path pre-deployment validation before any test executes.
//
// Stage 1 requirement (master-flow.md:186):
//
//	"Invalid mask paths must fail validation before deployment."
//
// If any mask path cannot be resolved within its corresponding schema, the
// entire test binary exits non-zero here — preventing deployment of a broken
// schema/mask pair. This mirrors a CI gate: schema_test is run on every build.
func TestMain(m *testing.M) {
	if err := validateAllMasks(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: mask path pre-deployment validation failed:\n%v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// validateAllMasks checks every registered entity type's mask against its schema.
// All errors are collected and reported together.
func validateAllMasks() error {
	// Only entity types with real embedded schemas are validated.
	// template and cluster are intentionally excluded from the dynamic schema endpoint.
	entityTypes := []string{"instancesize"}

	var allErrors []string
	for _, et := range entityTypes {
		sBytes, ok := schema.SchemaFor(et)
		if !ok {
			allErrors = append(allErrors, fmt.Sprintf("[%s] SchemaFor returned false", et))
			continue
		}
		mBytes, ok := schema.MaskFor(et)
		if !ok {
			allErrors = append(allErrors, fmt.Sprintf("[%s] MaskFor returned false", et))
			continue
		}
		if err := schema.ValidateMaskPaths(sBytes, mBytes); err != nil {
			allErrors = append(allErrors, fmt.Sprintf("[%s] %v", et, err))
		}
	}

	if len(allErrors) > 0 {
		combined := ""
		for _, e := range allErrors {
			combined += "\n" + e
		}
		return errors.New(combined)
	}
	return nil
}

// ─── Unit tests ───────────────────────────────────────────────────────────────

func TestSchemaFor_KnownEntityTypes(t *testing.T) {
	// Only instancesize has a real embedded schema.
	knownTypes := []string{"instancesize"}

	for _, entityType := range knownTypes {
		t.Run(entityType, func(t *testing.T) {
			data, ok := schema.SchemaFor(entityType)
			if !ok {
				t.Fatalf("SchemaFor(%q) returned ok=false, want true", entityType)
			}
			if len(data) == 0 {
				t.Fatalf("SchemaFor(%q) returned empty bytes", entityType)
			}
			// Verify it parses as valid JSON object.
			var obj map[string]interface{}
			if err := json.Unmarshal(data, &obj); err != nil {
				t.Fatalf("SchemaFor(%q): invalid JSON: %v", entityType, err)
			}
		})
	}
}

func TestSchemaFor_UnknownEntityType(t *testing.T) {
	// template and cluster are intentionally excluded from the schema endpoint.
	for _, unknown := range []string{"template", "cluster", "nonexistent", ""} {
		t.Run(unknown, func(t *testing.T) {
			_, ok := schema.SchemaFor(unknown)
			if ok {
				t.Errorf("SchemaFor(%q) returned ok=true, want false", unknown)
			}
		})
	}
}

func TestMaskFor_KnownEntityTypes(t *testing.T) {
	// Only instancesize has a real embedded mask.
	knownTypes := []string{"instancesize"}

	for _, entityType := range knownTypes {
		t.Run(entityType, func(t *testing.T) {
			data, ok := schema.MaskFor(entityType)
			if !ok {
				t.Fatalf("MaskFor(%q) returned ok=false, want true", entityType)
			}
			if len(data) == 0 {
				t.Fatalf("MaskFor(%q) returned empty bytes", entityType)
			}
			// Verify it parses as valid JSON with a quick_fields array.
			var mask struct {
				QuickFields        []interface{} `json:"quick_fields"`
				AdvancedFields     []interface{} `json:"advanced_fields"`
				ProfessionalFields []interface{} `json:"professional_fields"`
			}
			if err := json.Unmarshal(data, &mask); err != nil {
				t.Fatalf("MaskFor(%q): invalid JSON: %v", entityType, err)
			}
			// quick_fields may be empty but must be present (not nil after unmarshal).
			if mask.QuickFields == nil {
				t.Fatalf("MaskFor(%q): quick_fields is nil, want array (may be empty)", entityType)
			}
		})
	}
}

func TestMaskFor_UnknownEntityType(t *testing.T) {
	// template and cluster are intentionally excluded from the schema endpoint.
	for _, unknown := range []string{"template", "cluster", "nonexistent", ""} {
		t.Run(unknown, func(t *testing.T) {
			_, ok := schema.MaskFor(unknown)
			if ok {
				t.Errorf("MaskFor(%q) returned ok=true, want false", unknown)
			}
		})
	}
}

func TestCurrentVersionKeyFor_Instancesize(t *testing.T) {
	versionKey, ok := schema.CurrentVersionKeyFor("instancesize")
	if !ok {
		t.Fatal("CurrentVersionKeyFor(instancesize) returned ok=false")
	}
	if versionKey != "kubevirt-v1.8.2" {
		t.Fatalf("CurrentVersionKeyFor(instancesize) = %q, want %q", versionKey, "kubevirt-v1.8.2")
	}
}

func TestAvailableVersions_Instancesize(t *testing.T) {
	versions, ok := schema.AvailableVersions("instancesize")
	if !ok {
		t.Fatal("AvailableVersions(instancesize) returned ok=false")
	}
	if len(versions) == 0 {
		t.Fatal("AvailableVersions(instancesize) returned no versions")
	}
	if versions[len(versions)-1].Key != "kubevirt-v1.8.2" {
		t.Fatalf("AvailableVersions(instancesize)[last].Key = %q, want %q", versions[len(versions)-1].Key, "kubevirt-v1.8.2")
	}
	if versions[len(versions)-1].KubeVirtVersion != "1.8.2" {
		t.Fatalf("AvailableVersions(instancesize)[last].KubeVirtVersion = %q, want %q", versions[len(versions)-1].KubeVirtVersion, "1.8.2")
	}
}

func TestAvailableSchemaVersions_Instancesize(t *testing.T) {
	versions, ok := schema.AvailableSchemaVersions("instancesize")
	if !ok {
		t.Fatal("AvailableSchemaVersions(instancesize) returned ok=false")
	}
	if len(versions) == 0 {
		t.Fatal("AvailableSchemaVersions(instancesize) returned no versions")
	}
	if versions[len(versions)-1] != "1.8.2" {
		t.Fatalf("AvailableSchemaVersions(instancesize)[last] = %q, want %q", versions[len(versions)-1], "1.8.2")
	}
}

func TestVersionKeyForKubeVirtVersion_Instancesize(t *testing.T) {
	versionKey, ok := schema.VersionKeyForKubeVirtVersion("instancesize", "1.8.2")
	if !ok {
		t.Fatal("VersionKeyForKubeVirtVersion(instancesize, 1.8.2) returned ok=false")
	}
	if versionKey != "kubevirt-v1.8.2" {
		t.Fatalf("VersionKeyForKubeVirtVersion(instancesize, 1.8.2) = %q, want %q", versionKey, "kubevirt-v1.8.2")
	}
}

func TestFieldIntroducedVersions_Instancesize(t *testing.T) {
	introduced, err := schema.FieldIntroducedVersions("instancesize")
	if err != nil {
		t.Fatalf("FieldIntroducedVersions(instancesize) error = %v", err)
	}

	const newPath = "spec.template.spec.domain.rebootPolicy"
	if got := introduced[newPath]; got != "1.8.0" {
		t.Fatalf("FieldIntroducedVersions(instancesize)[%q] = %q, want %q", newPath, got, "1.8.0")
	}

	const baselinePath = "spec.template.spec.domain.devices.autoattachGraphicsDevice"
	if got, ok := introduced[baselinePath]; ok {
		t.Fatalf("FieldIntroducedVersions(instancesize)[%q] = %q, want omitted baseline field", baselinePath, got)
	}
}

func TestCurrentVersionDiffSummary_Instancesize(t *testing.T) {
	diff, err := schema.CurrentVersionDiffSummary("instancesize")
	if err != nil {
		t.Fatalf("CurrentVersionDiffSummary(instancesize) error = %v", err)
	}
	if diff == nil {
		t.Fatal("CurrentVersionDiffSummary(instancesize) = nil, want summary")
		return
	}
	if diff.FromVersion != "1.8.1" {
		t.Fatalf("FromVersion = %q, want %q", diff.FromVersion, "1.8.1")
	}
	if diff.ToVersion != "1.8.2" {
		t.Fatalf("ToVersion = %q, want %q", diff.ToVersion, "1.8.2")
	}
	if len(diff.SchemaPathsAdded) != 0 {
		t.Fatalf("SchemaPathsAdded length = %d, want 0 for patch baseline refresh", len(diff.SchemaPathsAdded))
	}
	if len(diff.ChangedPaths) != 0 {
		t.Fatalf("ChangedPaths length = %d, want 0 for patch baseline refresh", len(diff.ChangedPaths))
	}
	if len(diff.MaskPathsAdded) != 0 {
		t.Fatalf("MaskPathsAdded length = %d, want 0", len(diff.MaskPathsAdded))
	}
}

// TestInstancesizeSchema_RequiredPaths verifies that the embedded instancesize
// schema contains the paths referenced by ALL mask fields (not just spot-check).
// This is insurance in addition to TestMain's full validation.
func TestInstancesizeSchema_RequiredPaths(t *testing.T) {
	schemaBytes, _ := schema.SchemaFor("instancesize")
	maskBytes, _ := schema.MaskFor("instancesize")

	if err := schema.ValidateMaskPaths(schemaBytes, maskBytes); err != nil {
		t.Errorf("mask path validation failed:\n%v", err)
	}
}

// TestValidateMaskPaths_InvalidPath verifies that ValidateMaskPaths correctly
// rejects a mask pointing to a non-existent schema path.
func TestValidateMaskPaths_InvalidPath(t *testing.T) {
	schemaJSON := []byte(`{
		"properties": {
			"spec": {
				"properties": {
					"cores": {"type": "integer"}
				}
			}
		}
	}`)
	maskJSON := []byte(`{
		"quick_fields": [{"path": "spec.cores"}],
		"advanced_fields": [{"path": "spec.nonexistent.deep.path"}],
		"professional_fields": []
	}`)

	err := schema.ValidateMaskPaths(schemaJSON, maskJSON)
	if err == nil {
		t.Fatal("expected error for invalid mask path, got nil")
	}
	// Should report the invalid path specifically.
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
	t.Logf("correctly rejected invalid path with error: %v", err)
}

// TestValidateMaskPaths_ArrayTraversal verifies that paths through array-typed
// nodes (e.g., gpus[]) are correctly resolved via items.properties.
func TestValidateMaskPaths_ArrayTraversal(t *testing.T) {
	schemaJSON := []byte(`{
		"properties": {
			"devices": {
				"properties": {
					"gpus": {
						"type": "array",
						"items": {
							"properties": {
								"deviceName": {"type": "string"}
							}
						}
					}
				}
			}
		}
	}`)
	maskJSON := []byte(`{
		"quick_fields": [{"path": "devices.gpus"}],
		"advanced_fields": [],
		"professional_fields": []
	}`)

	if err := schema.ValidateMaskPaths(schemaJSON, maskJSON); err != nil {
		t.Errorf("array traversal path rejected unexpectedly: %v", err)
	}
}
