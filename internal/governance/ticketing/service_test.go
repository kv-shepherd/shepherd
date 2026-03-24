package ticketing

import (
	"encoding/json"
	"testing"

	"kv-shepherd.io/shepherd/ent"
)

func TestParseVMCreatePayload(t *testing.T) {
	raw, err := json.Marshal(map[string]interface{}{
		"service_id":       "svc-1",
		"template_id":      "tpl-1",
		"namespace":        "ns-1",
		"requester_id":     "user-1",
		"instance_size_id": "size-1",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	payload, err := parseVMCreatePayload(raw)
	if err != nil {
		t.Fatalf("parse payload error: %v", err)
	}
	if payload.TemplateID != "tpl-1" {
		t.Fatalf("template id mismatch: got %q", payload.TemplateID)
	}
}

func TestParseVMCreatePayload_MissingFieldsRejected(t *testing.T) {
	raw, err := json.Marshal(map[string]interface{}{
		"service_id": "svc-1",
		// namespace/requester/template/instance_size intentionally missing
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, err := parseVMCreatePayload(raw); err == nil {
		t.Fatal("parseVMCreatePayload expected validation error, got nil")
	}
}

func TestResolveEffectiveSelectionIDs(t *testing.T) {
	templateID, instanceSizeID := resolveEffectiveSelectionIDs("tpl-a", "size-a", map[string]interface{}{
		"template_id":      "tpl-b",
		"instance_size_id": "size-b",
	})
	if templateID != "tpl-b" {
		t.Fatalf("template id mismatch: got %q", templateID)
	}
	if instanceSizeID != "size-b" {
		t.Fatalf("instance size id mismatch: got %q", instanceSizeID)
	}
}

func TestBuildTemplateSnapshot(t *testing.T) {
	tpl := &ent.Template{
		ID:           "tpl-1",
		Name:         "ubuntu",
		DisplayName:  "Ubuntu",
		Description:  "Ubuntu template",
		SourceType:   "cdi_pvc_clone",
		PvcName:      "ubuntu-base",
		PvcNamespace: "golden-images",
		CloudInit:    "#cloud-config\nusers:\n  - name: admin",
		OsFamily:     "linux",
		OsVersion:    "22.04",
		Enabled:      true,
		CreatedBy:    "admin",
	}

	snapshot := buildTemplateSnapshot(tpl)
	if snapshot["id"] != "tpl-1" {
		t.Fatalf("snapshot id mismatch: got %v", snapshot["id"])
	}
	if snapshot["source_type"] != "cdi_pvc_clone" {
		t.Fatalf("snapshot source type mismatch: got %v", snapshot["source_type"])
	}
	if snapshot["pvc_name"] != "ubuntu-base" {
		t.Fatalf("snapshot pvc name mismatch: got %v", snapshot["pvc_name"])
	}
	if snapshot["pvc_namespace"] != "golden-images" {
		t.Fatalf("snapshot pvc namespace mismatch: got %v", snapshot["pvc_namespace"])
	}
	if snapshot["cloud_init"] != "#cloud-config\nusers:\n  - name: admin" {
		t.Fatalf("snapshot cloud_init mismatch: got %v", snapshot["cloud_init"])
	}
}
