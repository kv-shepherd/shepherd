package vmmutationplan

import (
	"testing"

	"kv-shepherd.io/shepherd/internal/domain"
)

func TestPlanVMResourceUpdatePatchDelegatesToProviderImplementation(t *testing.T) {
	memoryLimit := 12.0
	plan, err := PlanVMResourceUpdatePatch("prod-ns", &domain.VM{
		Name:      "vm-1",
		Cluster:   "cluster-a",
		Status:    domain.VMStatusRunning,
		Namespace: "prod-ns",
		Spec: domain.VMSpec{
			CPU:             8,
			MemoryGi:        8,
			CPURequest:      8,
			MemoryRequestGi: 8,
		},
	}, VMLiveUpdateTargets{
		MemoryGi:        &memoryLimit,
		MemoryRequestGi: &memoryLimit,
	})
	if err != nil {
		t.Fatalf("PlanVMResourceUpdatePatch returned error: %v", err)
	}
	if plan == nil || plan.Mutation == nil {
		t.Fatal("PlanVMResourceUpdatePatch returned nil plan or mutation")
	}
	if plan.ApplyMode != "live" {
		t.Fatalf("PlanVMResourceUpdatePatch ApplyMode = %q, want %q", plan.ApplyMode, "live")
	}
}
