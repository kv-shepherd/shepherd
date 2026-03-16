package handlers

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestBuildClusterCompatibilityFilter_IncludesExplicitRootVolumeMode(t *testing.T) {
	t.Parallel()

	input, hasFilter := buildClusterCompatibilityFilter(generated.ListClustersParams{
		Namespace:             "prod-a",
		TemplateId:            "tpl-1",
		InstanceSizeId:        "sz-1",
		SelectedStorageClass:  "rook-ceph",
		SelectedDvAccessModes: []string{"ReadWriteMany"},
		SelectedDvVolumeMode:  generated.ListClustersParamsSelectedDvVolumeMode("Block"),
	})

	if !hasFilter {
		t.Fatal("hasFilter = false, want true")
	}
	if input.Namespace != "prod-a" {
		t.Fatalf("namespace = %q, want prod-a", input.Namespace)
	}
	if input.TemplateID != "tpl-1" {
		t.Fatalf("template_id = %q, want tpl-1", input.TemplateID)
	}
	if input.InstanceSizeID != "sz-1" {
		t.Fatalf("instance_size_id = %q, want sz-1", input.InstanceSizeID)
	}
	if input.StorageClass != "rook-ceph" {
		t.Fatalf("storage_class = %q, want rook-ceph", input.StorageClass)
	}
	if len(input.DVAccessModes) != 1 || input.DVAccessModes[0] != "ReadWriteMany" {
		t.Fatalf("dv_access_modes = %#v, want [ReadWriteMany]", input.DVAccessModes)
	}
	if input.DVVolumeMode != "Block" {
		t.Fatalf("dv_volume_mode = %q, want Block", input.DVVolumeMode)
	}
}
