package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVMSpecJSON_ExposeRootVolumeDVFields(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(VMSpec{
		Name:          "vm-a",
		CPU:           4,
		MemoryGi:      8,
		DVAccessModes: []string{"ReadWriteMany"},
		DVVolumeMode:  "Block",
	})
	if err != nil {
		t.Fatalf("Marshal(VMSpec) error = %v", err)
	}

	body := string(raw)
	if !strings.Contains(body, `"dv_access_modes":["ReadWriteMany"]`) {
		t.Fatalf("Marshal(VMSpec) = %s, want dv_access_modes", body)
	}
	if !strings.Contains(body, `"dv_volume_mode":"Block"`) {
		t.Fatalf("Marshal(VMSpec) = %s, want dv_volume_mode", body)
	}
}

func TestVMSpecJSON_HidesLiveUpdateInternalFields(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(VMSpec{
		Name:                     "vm-a",
		CPU:                      4,
		MemoryGi:                 8,
		RootDataVolumeName:       "rootdisk",
		RootVolumeUsesPVCSpec:    true,
		DiskHotplugSupported:     true,
		CurrentCPUSockets:        2,
		CurrentCPUCoresPerSocket: 2,
		CurrentCPUThreads:        1,
	})
	if err != nil {
		t.Fatalf("Marshal(VMSpec) error = %v", err)
	}

	body := string(raw)
	for _, forbidden := range []string{
		"rootdisk",
		"RootDataVolumeName",
		"root_volume_uses_pvc_spec",
		"disk_hotplug_supported",
		"current_cpu_sockets",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Marshal(VMSpec) leaked internal field %q: %s", forbidden, body)
		}
	}
}

func TestVMModifyPayloadJSON_PreservesTargetResources(t *testing.T) {
	t.Parallel()

	targetCPU := 4.0
	targetMemory := 8.0
	targetDisk := 40

	raw, err := json.Marshal(VMModifyPayload{
		VMID:            "vm-1",
		VMName:          "vm-one",
		ClusterID:       "cluster-a",
		Namespace:       "prod-ns",
		Actor:           "owner-1",
		CurrentCPUCores: 2,
		CurrentMemoryGi: 4,
		CurrentDiskGB:   20,
		TargetCPUCores:  &targetCPU,
		TargetMemoryGi:  &targetMemory,
		TargetDiskGB:    &targetDisk,
	})
	if err != nil {
		t.Fatalf("Marshal(VMModifyPayload) error = %v", err)
	}

	body := string(raw)
	for _, want := range []string{
		`"target_cpu_cores":4`,
		`"target_memory_gi":8`,
		`"target_disk_gb":40`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Marshal(VMModifyPayload) = %s, want %s", body, want)
		}
	}
}
