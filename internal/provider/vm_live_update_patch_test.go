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
	patchPayload := string(patch.Payload)

	for _, want := range []string{
		"\"sockets\":2",
		"\"cpu\":\"4\"",
		"\"guest\":\"8Gi\"",
		"\"memory\":\"8Gi\"",
		"\"name\":\"rootdisk\"",
		"\"storage\":\"40Gi\"",
	} {
		if !strings.Contains(patchPayload, want) {
			t.Fatalf("patch missing %q:\n%s", want, patchPayload)
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

func TestRenderVMResourceUpdatePatch_AllowsStoppedCPUAndMemoryResize(t *testing.T) {
	t.Parallel()

	targetCPU := 12.0
	targetMemory := 2.0

	patch, err := RenderVMResourceUpdatePatch("prod-ns", &domain.VM{
		Name:      "vm-a",
		Namespace: "prod-ns",
		Status:    domain.VMStatusStopped,
		Spec: domain.VMSpec{
			CPU:                      8,
			MemoryGi:                 4,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 8,
			CurrentCPUThreads:        1,
		},
	}, VMLiveUpdateTargets{
		CPUCores: &targetCPU,
		MemoryGi: &targetMemory,
	})
	if err != nil {
		t.Fatalf("RenderVMResourceUpdatePatch returned error: %v", err)
	}
	patchPayload := string(patch.Payload)

	for _, want := range []string{
		"\"sockets\":1",
		"\"cores\":12",
		"\"threads\":1",
		"\"cpu\":\"12\"",
		"\"guest\":\"2Gi\"",
		"\"memory\":\"2Gi\"",
	} {
		if !strings.Contains(patchPayload, want) {
			t.Fatalf("patch missing %q:\n%s", want, patchPayload)
		}
	}
}

func TestPlanVMResourceUpdatePatch_RunningShrinkFallsBackToRestartRequired(t *testing.T) {
	t.Parallel()

	targetCPU := 4.0
	targetMemory := 2.0

	plan, err := PlanVMResourceUpdatePatch("prod-ns", &domain.VM{
		Name:      "vm-a",
		Namespace: "prod-ns",
		Status:    domain.VMStatusRunning,
		Spec: domain.VMSpec{
			CPU:                      8,
			MemoryGi:                 4,
			CurrentCPUSockets:        1,
			CurrentCPUCoresPerSocket: 8,
			CurrentCPUThreads:        1,
		},
	}, VMLiveUpdateTargets{
		CPUCores: &targetCPU,
		MemoryGi: &targetMemory,
	})
	if err != nil {
		t.Fatalf("PlanVMResourceUpdatePatch returned error: %v", err)
	}
	if !plan.RequiresRestart {
		t.Fatal("plan.RequiresRestart = false, want true")
	}
	if plan.ApplyMode != "restart_required" {
		t.Fatalf("plan.ApplyMode = %q, want %q", plan.ApplyMode, "restart_required")
	}
	for _, want := range []string{
		"\"cpu\":\"4\"",
		"\"guest\":\"2Gi\"",
		"\"memory\":\"2Gi\"",
	} {
		if !strings.Contains(string(plan.Mutation.Payload), want) {
			t.Fatalf("plan.Mutation.Payload missing %q:\n%s", want, string(plan.Mutation.Payload))
		}
	}
	if strings.Contains(string(plan.Mutation.Payload), "\"requests\"") {
		t.Fatalf("plan.Mutation.Payload unexpectedly includes requests block:\n%s", string(plan.Mutation.Payload))
	}
}
