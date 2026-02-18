package provider

import (
	"testing"

	k8sv1 "k8s.io/api/core/v1"

	"kv-shepherd.io/shepherd/internal/domain"
)

func TestBuildVMFromSpec_ValidContainerDisk(t *testing.T) {
	spec := &domain.VMSpec{
		Name:     "vm-01",
		CPU:      4,
		MemoryMB: 8192,
		DiskGB:   20,
		Image:    "docker.io/kubevirt/centos:7",
		Labels: map[string]string{
			"env": "test",
		},
	}

	vm, err := buildVMFromSpec("test-ns", spec)
	if err != nil {
		t.Fatalf("buildVMFromSpec returned error: %v", err)
	}

	if vm.Name != "vm-01" {
		t.Fatalf("vm name mismatch: got %q", vm.Name)
	}
	if vm.Namespace != "test-ns" {
		t.Fatalf("vm namespace mismatch: got %q", vm.Namespace)
	}
	if vm.Spec.Template == nil {
		t.Fatalf("vm template is nil")
	}

	cpu := vm.Spec.Template.Spec.Domain.Resources.Requests[k8sv1.ResourceCPU]
	if cpu.String() != "4" {
		t.Fatalf("cpu request mismatch: got %q", cpu.String())
	}
	mem := vm.Spec.Template.Spec.Domain.Resources.Requests[k8sv1.ResourceMemory]
	if mem.String() != "8Gi" {
		t.Fatalf("memory request mismatch: got %q", mem.String())
	}

	volumes := vm.Spec.Template.Spec.Volumes
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes (root+data), got %d", len(volumes))
	}
	disks := vm.Spec.Template.Spec.Domain.Devices.Disks
	if len(disks) != 2 {
		t.Fatalf("expected 2 disks (root+data), got %d", len(disks))
	}

	rootFound := false
	dataFound := false
	for _, v := range volumes {
		switch v.Name {
		case "rootdisk":
			rootFound = v.VolumeSource.ContainerDisk != nil
		case "datadisk":
			dataFound = v.VolumeSource.EmptyDisk != nil
		}
	}
	if !rootFound {
		t.Fatalf("rootdisk container source missing")
	}
	if !dataFound {
		t.Fatalf("datadisk emptyDisk source missing")
	}
}

func TestBuildVMFromSpec_ValidPVCSource(t *testing.T) {
	spec := &domain.VMSpec{
		Name:     "vm-pvc",
		CPU:      2,
		MemoryMB: 4096,
		Image:    "pvc:base-os-disk",
	}

	vm, err := buildVMFromSpec("prod-ns", spec)
	if err != nil {
		t.Fatalf("buildVMFromSpec returned error: %v", err)
	}

	if vm.Spec.Template == nil || len(vm.Spec.Template.Spec.Volumes) == 0 {
		t.Fatalf("expected at least one volume")
	}
	root := vm.Spec.Template.Spec.Volumes[0]
	if root.Name != "rootdisk" {
		t.Fatalf("expected rootdisk, got %q", root.Name)
	}
	if root.VolumeSource.PersistentVolumeClaim == nil {
		t.Fatalf("expected rootdisk persistentVolumeClaim source")
	}
	if root.VolumeSource.PersistentVolumeClaim.ClaimName != "base-os-disk" {
		t.Fatalf("pvc claim mismatch: got %q", root.VolumeSource.PersistentVolumeClaim.ClaimName)
	}
}

func TestBuildVMFromSpec_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name string
		spec *domain.VMSpec
	}{
		{name: "nil spec", spec: nil},
		{name: "missing name", spec: &domain.VMSpec{CPU: 1, MemoryMB: 512, Image: "img"}},
		{name: "missing cpu", spec: &domain.VMSpec{Name: "vm", MemoryMB: 512, Image: "img"}},
		{name: "missing memory", spec: &domain.VMSpec{Name: "vm", CPU: 1, Image: "img"}},
		{name: "missing image", spec: &domain.VMSpec{Name: "vm", CPU: 1, MemoryMB: 512}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildVMFromSpec("ns", tc.spec)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestBuildVMFromSpec_AppliesSpecOverrides(t *testing.T) {
	spec := &domain.VMSpec{
		Name:     "vm-adv",
		CPU:      2,
		MemoryMB: 2048,
		Image:    "docker.io/kubevirt/fedora:40",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.cpu.cores":                          float64(6),
			"spec.template.spec.domain.memory.hugepages.pageSize":          "2Mi",
			"spec.template.spec.domain.cpu.dedicatedCpuPlacement":          true,
			"spec.template.spec.domain.devices.gpus":                       []interface{}{map[string]interface{}{"name": "gpu0", "deviceName": "nvidia.com/A10"}},
			"spec.template.spec.domain.resources.requests.memory":          "3072Mi",
			"spec.template.spec.domain.resources.limits.memory":            "4096Mi",
			"spec.template.spec.domain.devices.networkInterfaceMultiqueue": true,
		},
	}

	vm, err := buildVMFromSpec("ns-1", spec)
	if err != nil {
		t.Fatalf("buildVMFromSpec returned error: %v", err)
	}
	if vm.Spec.Template == nil {
		t.Fatalf("vm template is nil")
	}
	if vm.Spec.Template.Spec.Domain.CPU == nil {
		t.Fatalf("vm cpu is nil")
	}
	if vm.Spec.Template.Spec.Domain.CPU.Cores != 6 {
		t.Fatalf("expected overridden cpu cores=6, got %d", vm.Spec.Template.Spec.Domain.CPU.Cores)
	}
	if !vm.Spec.Template.Spec.Domain.CPU.DedicatedCPUPlacement {
		t.Fatalf("expected dedicatedCpuPlacement=true from spec_overrides")
	}
	if vm.Spec.Template.Spec.Domain.Memory == nil ||
		vm.Spec.Template.Spec.Domain.Memory.Hugepages == nil ||
		vm.Spec.Template.Spec.Domain.Memory.Hugepages.PageSize != "2Mi" {
		t.Fatalf("expected hugepages.pageSize=2Mi from spec_overrides")
	}
	if len(vm.Spec.Template.Spec.Domain.Devices.GPUs) != 1 {
		t.Fatalf("expected one GPU override, got %d", len(vm.Spec.Template.Spec.Domain.Devices.GPUs))
	}
	if vm.Spec.Template.Spec.Domain.Devices.GPUs[0].Name != "gpu0" {
		t.Fatalf("expected gpu name gpu0, got %q", vm.Spec.Template.Spec.Domain.Devices.GPUs[0].Name)
	}
	memReq := vm.Spec.Template.Spec.Domain.Resources.Requests[k8sv1.ResourceMemory]
	if memReq.String() != "3Gi" {
		t.Fatalf("expected memory request 3Gi after override, got %q", memReq.String())
	}
	memLimit := vm.Spec.Template.Spec.Domain.Resources.Limits[k8sv1.ResourceMemory]
	if memLimit.String() != "4Gi" {
		t.Fatalf("expected memory limit 4Gi after override, got %q", memLimit.String())
	}
	if vm.Spec.Template.Spec.Domain.Devices.NetworkInterfaceMultiQueue == nil ||
		!*vm.Spec.Template.Spec.Domain.Devices.NetworkInterfaceMultiQueue {
		t.Fatalf("expected networkInterfaceMultiqueue=true from spec_overrides")
	}
}

func TestBuildVMFromSpec_RejectsInvalidSpecOverridePath(t *testing.T) {
	spec := &domain.VMSpec{
		Name:     "vm-invalid",
		CPU:      2,
		MemoryMB: 1024,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"metadata.labels.foo": "bar",
		},
	}

	_, err := buildVMFromSpec("ns", spec)
	if err == nil {
		t.Fatalf("expected error for invalid override path, got nil")
	}
}
