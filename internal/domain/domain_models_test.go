package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVMSpecJSON_ExcludesRenderedYAML(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(VMSpec{
		Name:         "vm-a",
		CPU:          2,
		MemoryGi:     4,
		StorageClass: "gold-sc",
		RenderedYAML: "kind: VirtualMachine",
	})
	if err != nil {
		t.Fatalf("Marshal(VMSpec) error = %v", err)
	}
	if strings.Contains(string(raw), "RenderedYAML") || strings.Contains(string(raw), "rendered_yaml") {
		t.Fatalf("Marshal(VMSpec) = %s, want RenderedYAML omitted", string(raw))
	}
	if !strings.Contains(string(raw), `"storage_class":"gold-sc"`) {
		t.Fatalf("Marshal(VMSpec) = %s, want storage_class present", string(raw))
	}
}

func TestProvisioningTypesJSON_ExposeVolumeModeAndStorageProfileFields(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(struct {
		PVC     PersistentVolumeClaim `json:"pvc"`
		Profile StorageProfile        `json:"profile"`
	}{
		PVC: PersistentVolumeClaim{
			Name:             "golden",
			Namespace:        "images",
			VolumeMode:       "Block",
			StorageClassName: "fast-sc",
		},
		Profile: StorageProfile{
			Name:              "fast-sc",
			CloneStrategy:     "copy",
			DefaultVolumeMode: "Filesystem",
		},
	})
	if err != nil {
		t.Fatalf("Marshal(provisioning models) error = %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"volume_mode":"Block"`) {
		t.Fatalf("Marshal(provisioning models) = %s, want volume_mode", body)
	}
	if !strings.Contains(body, `"clone_strategy":"copy"`) {
		t.Fatalf("Marshal(provisioning models) = %s, want clone_strategy", body)
	}
}
