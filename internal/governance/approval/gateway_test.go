package approval

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
		ID:          "tpl-1",
		Name:        "ubuntu",
		DisplayName: "Ubuntu",
		Description: "Ubuntu template",
		Version:     3,
		OsFamily:    "linux",
		OsVersion:   "22.04",
		Enabled:     true,
		CreatedBy:   "admin",
		Spec: map[string]interface{}{
			"image_source": map[string]interface{}{
				"image": "docker.io/ubuntu:22.04",
			},
		},
	}

	snapshot := buildTemplateSnapshot(tpl)
	if snapshot["id"] != "tpl-1" {
		t.Fatalf("snapshot id mismatch: got %v", snapshot["id"])
	}
	if snapshot["version"] != 3 {
		t.Fatalf("snapshot version mismatch: got %v", snapshot["version"])
	}
	spec, ok := snapshot["spec"].(map[string]interface{})
	if !ok || len(spec) == 0 {
		t.Fatalf("snapshot spec missing")
	}
}
