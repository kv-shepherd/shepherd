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
