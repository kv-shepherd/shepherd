package provider

import (
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/domain"
)

func TestRenderVMLiveUpdatePatch_RendersCPUAndMemoryAndDiskExpansion(t *testing.T) {
	t.Parallel()

	targetCPU := 4.0
	targetMemory := 8.0
	targetDisk := 40

	patch, err := RenderVMLiveUpdatePatch("prod-ns", &domain.VM{
		Name:      "vm-a",
		Namespace: "prod-ns",
		Spec: domain.VMSpec{
			CPU:                      2,
			MemoryGi:                 4,
			DiskGB:                   20,
			RootDataVolumeName:       "rootdisk",
			DiskHotplugSupported:     true,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 2,
			CurrentCPUThreads:        1,
		},
	}, VMLiveUpdateTargets{
		CPUCores: &targetCPU,
		MemoryGi: &targetMemory,
		DiskGB:   &targetDisk,
	})
	if err != nil {
		t.Fatalf("RenderVMLiveUpdatePatch returned error: %v", err)
	}

	for _, want := range []string{
		"name: vm-a",
		"namespace: prod-ns",
		"sockets: 2",
		"cpu: \"4\"",
		"guest: 8Gi",
		"memory: 8Gi",
		"name: rootdisk",
		"storage: 40Gi",
	} {
		if !strings.Contains(patch, want) {
			t.Fatalf("patch missing %q:\n%s", want, patch)
		}
	}
}

func TestRenderVMLiveUpdatePatch_RejectsIncompatibleCPUTarget(t *testing.T) {
	t.Parallel()

	targetCPU := 3.0

	_, err := RenderVMLiveUpdatePatch("prod-ns", &domain.VM{
		Name:      "vm-a",
		Namespace: "prod-ns",
		Spec: domain.VMSpec{
			CPU:                      2,
			MemoryGi:                 4,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 2,
			CurrentCPUThreads:        1,
		},
	}, VMLiveUpdateTargets{
		CPUCores: &targetCPU,
	})
	if err == nil {
		t.Fatal("expected incompatible CPU topology error, got nil")
	}
	if !strings.Contains(err.Error(), "socket increments") {
		t.Fatalf("error = %q, want socket increments hint", err)
	}
}

func TestResolveVMLiveCPUHotplugSupport_UsesSocketCapacity(t *testing.T) {
	t.Parallel()

	currentTotal, perSocketIncrement, err := ResolveVMLiveCPUHotplugSupport(&domain.VM{
		Spec: domain.VMSpec{
			CurrentCPUSockets:        2,
			CurrentCPUCoresPerSocket: 2,
			CurrentCPUThreads:        2,
		},
	})
	if err != nil {
		t.Fatalf("ResolveVMLiveCPUHotplugSupport returned error: %v", err)
	}
	if currentTotal != 8 {
		t.Fatalf("currentTotal = %d, want 8", currentTotal)
	}
	if perSocketIncrement != 4 {
		t.Fatalf("perSocketIncrement = %d, want 4", perSocketIncrement)
	}
}
