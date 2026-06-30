package jobs

import (
	"slices"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/service"
)

func TestExtractTemplateImage(t *testing.T) {
	testCases := []struct {
		name        string
		spec        map[string]interface{}
		expectImage string
		expectErr   bool
	}{
		{
			name: "direct image_source containerdisk",
			spec: map[string]interface{}{
				"image_source": map[string]interface{}{
					"type":  "containerdisk",
					"image": "docker.io/kubevirt/centos:7",
				},
			},
			expectImage: "docker.io/kubevirt/centos:7",
		},
		{
			name: "pvc source",
			spec: map[string]interface{}{
				"image_source": map[string]interface{}{
					"type":     "pvc",
					"pvc_name": "centos-base",
				},
			},
			expectImage: "clone-pvc:centos-base",
		},
		{
			name: "legacy direct pvc volume rejected",
			spec: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "rootfs",
									"persistentVolumeClaim": map[string]interface{}{
										"claimName": "shared-rootfs",
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "volumes containerDisk fallback",
			spec: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "rootfs",
									"containerDisk": map[string]interface{}{
										"image": "quay.io/kubevirt/fedora:40",
									},
								},
							},
						},
					},
				},
			},
			expectImage: "quay.io/kubevirt/fedora:40",
		},
		{
			name:      "missing image source",
			spec:      map[string]interface{}{"foo": "bar"},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			image, err := extractTemplateImage(tc.spec)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if image != tc.expectImage {
				t.Fatalf("image mismatch: got %q want %q", image, tc.expectImage)
			}
		})
	}
}

func TestExtractTemplateImageFromEnt(t *testing.T) {
	tests := []struct {
		name      string
		tpl       *ent.Template
		wantImage string
		wantErr   bool
	}{
		{
			name: "image url preferred",
			tpl: &ent.Template{
				ID:         "tpl-1",
				SourceType: "containerdisk",
				ImageURL:   "quay.io/containerdisks/ubuntu:22.04",
			},
			wantImage: "quay.io/containerdisks/ubuntu:22.04",
		},
		{
			name: "pvc fallback",
			tpl: &ent.Template{
				ID:         "tpl-2",
				SourceType: "cdi_pvc_clone",
				PvcName:    "golden-pvc",
			},
			wantImage: "clone-pvc:golden-pvc",
		},
		{
			name: "cdi image import source",
			tpl: &ent.Template{
				ID:         "tpl-3",
				SourceType: "cdi_image_import",
				ImageURL:   "quay.io/containerdisks/fedora:40",
			},
			wantImage: "import-image:docker://quay.io/containerdisks/fedora:40",
		},
		{
			name: "missing source rejected",
			tpl: &ent.Template{
				ID: "tpl-4",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractTemplateImageFromEnt(tc.tpl)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantImage {
				t.Fatalf("image mismatch: got %q want %q", got, tc.wantImage)
			}
		})
	}
}

func TestExtractTemplateImageFromSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		snapshot  map[string]interface{}
		wantImage string
		wantErr   bool
	}{
		{
			name: "adr0036 image source",
			snapshot: map[string]interface{}{
				"source_type": "containerdisk",
				"image_url":   "quay.io/containerdisks/fedora:40",
			},
			wantImage: "quay.io/containerdisks/fedora:40",
		},
		{
			name: "adr0036 pvc source",
			snapshot: map[string]interface{}{
				"source_type": "cdi_pvc_clone",
				"pvc_name":    "fedora-golden",
			},
			wantImage: "clone-pvc:fedora-golden",
		},
		{
			name: "adr0036 pvc source with namespace",
			snapshot: map[string]interface{}{
				"source_type":   "cdi_pvc_clone",
				"pvc_name":      "fedora-golden",
				"pvc_namespace": "golden-images",
			},
			wantImage: "clone-pvc:golden-images/fedora-golden",
		},
		{
			name: "adr0036 cdi image import source",
			snapshot: map[string]interface{}{
				"source_type": "cdi_image_import",
				"image_url":   "docker://quay.io/containerdisks/fedora:40",
			},
			wantImage: "import-image:docker://quay.io/containerdisks/fedora:40",
		},
		{
			name: "legacy spec fallback",
			snapshot: map[string]interface{}{
				"image_source": map[string]interface{}{
					"type":  "containerdisk",
					"image": "docker.io/kubevirt/centos:7",
				},
			},
			wantImage: "docker.io/kubevirt/centos:7",
		},
		{
			name: "invalid snapshot rejected",
			snapshot: map[string]interface{}{
				"source_type": "image",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractTemplateImageFromSnapshot(tc.snapshot)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantImage {
				t.Fatalf("image mismatch: got %q want %q", got, tc.wantImage)
			}
		})
	}
}

func TestExtractTemplateCloudInitFromSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		snapshot  map[string]interface{}
		wantValue string
		wantFound bool
	}{
		{
			name: "snapshot contains cloud_init",
			snapshot: map[string]interface{}{
				"cloud_init": "#cloud-config\nusers:\n  - name: admin",
			},
			wantValue: "#cloud-config\nusers:\n  - name: admin",
			wantFound: true,
		},
		{
			name: "snapshot contains empty cloud_init",
			snapshot: map[string]interface{}{
				"cloud_init": "",
			},
			wantValue: "",
			wantFound: true,
		},
		{
			name: "snapshot missing cloud_init",
			snapshot: map[string]interface{}{
				"source_type": "containerdisk",
			},
			wantValue: "",
			wantFound: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, found := extractTemplateCloudInitFromSnapshot(tc.snapshot)
			if found != tc.wantFound {
				t.Fatalf("found mismatch: got %v want %v", found, tc.wantFound)
			}
			if got != tc.wantValue {
				t.Fatalf("value mismatch: got %q want %q", got, tc.wantValue)
			}
		})
	}
}

func TestResolveEffectiveSelectionIDs_DefaultsToPayload(t *testing.T) {
	t.Parallel()

	templateID, instanceSizeID := resolveEffectiveSelectionIDs(
		domain.VMCreationPayload{
			TemplateID:     "tpl-1",
			InstanceSizeID: "size-1",
		},
		nil,
	)

	if templateID != "tpl-1" {
		t.Fatalf("templateID = %q, want tpl-1", templateID)
	}
	if instanceSizeID != "size-1" {
		t.Fatalf("instanceSizeID = %q, want size-1", instanceSizeID)
	}
}

func TestResolveEffectiveSelectionIDs(t *testing.T) {
	payload := domain.VMCreationPayload{
		TemplateID:     "tpl-A",
		InstanceSizeID: "size-A",
	}
	templateID, instanceSizeID := resolveEffectiveSelectionIDs(payload, map[string]interface{}{
		"template_id":      "tpl-B",
		"instance_size_id": "size-B",
	})

	if templateID != "tpl-B" {
		t.Fatalf("templateID mismatch: got %q", templateID)
	}
	if instanceSizeID != "size-B" {
		t.Fatalf("instanceSizeID mismatch: got %q", instanceSizeID)
	}
}

func TestApplyInstanceSizeSnapshotOverrides_UsesCanonicalMemoryGiOnly(t *testing.T) {
	cpu := 2.0
	cpuRequest := 1.0
	mem := 2.0
	memoryRequestGi := 1.0
	disk := 10
	snapshot := map[string]interface{}{
		"memory_gi":         8.0,
		"memory_request_gi": 4.0,
		"cpu_cores":         4.0,
		"cpu_request":       2.0,
		"disk_gb":           80,
	}

	applyInstanceSizeSnapshotOverrides(&cpu, &cpuRequest, &mem, &memoryRequestGi, &disk, snapshot)

	if cpu != 4.0 {
		t.Fatalf("cpu mismatch: got %.1f want 4.0", cpu)
	}
	if cpuRequest != 2.0 {
		t.Fatalf("cpuRequest mismatch: got %.1f want 2.0", cpuRequest)
	}
	if mem != 8.0 {
		t.Fatalf("memoryGi mismatch: got %.1f want 8.0", mem)
	}
	if memoryRequestGi != 4.0 {
		t.Fatalf("memoryRequestGi mismatch: got %.1f want 4.0", memoryRequestGi)
	}
	if disk != 80 {
		t.Fatalf("disk mismatch: got %d want 80", disk)
	}
}

func TestApplyInstanceSizeSnapshotOverrides_NormalizesMissingLegacyRequestsToSnapshotLimits(t *testing.T) {
	cpu := 8.0
	cpuRequest := 8.0
	mem := 16.0
	memoryRequestGi := 16.0
	disk := 10
	snapshot := map[string]interface{}{
		"cpu_cores": 4.0,
		"memory_gi": 8.0,
	}

	applyInstanceSizeSnapshotOverrides(&cpu, &cpuRequest, &mem, &memoryRequestGi, &disk, snapshot)

	if cpu != 4.0 {
		t.Fatalf("cpu mismatch: got %.1f want 4.0", cpu)
	}
	if cpuRequest != 4.0 {
		t.Fatalf("cpuRequest mismatch: got %.1f want 4.0", cpuRequest)
	}
	if mem != 8.0 {
		t.Fatalf("memoryGi mismatch: got %.1f want 8.0", mem)
	}
	if memoryRequestGi != 8.0 {
		t.Fatalf("memoryRequestGi mismatch: got %.1f want 8.0", memoryRequestGi)
	}
}

func TestApplyInstanceSizeSnapshotOverrides_PreservesExplicitZeroRequestsForValidation(t *testing.T) {
	cpu := 8.0
	cpuRequest := 8.0
	mem := 16.0
	memoryRequestGi := 16.0
	disk := 10
	snapshot := map[string]interface{}{
		"cpu_cores":         4.0,
		"cpu_request":       0.0,
		"memory_gi":         8.0,
		"memory_request_gi": 0.0,
	}

	applyInstanceSizeSnapshotOverrides(&cpu, &cpuRequest, &mem, &memoryRequestGi, &disk, snapshot)

	if cpu != 4.0 {
		t.Fatalf("cpu mismatch: got %.1f want 4.0", cpu)
	}
	if cpuRequest != 0.0 {
		t.Fatalf("cpuRequest mismatch: got %.1f want 0.0", cpuRequest)
	}
	if mem != 8.0 {
		t.Fatalf("memoryGi mismatch: got %.1f want 8.0", mem)
	}
	if memoryRequestGi != 0.0 {
		t.Fatalf("memoryRequestGi mismatch: got %.1f want 0.0", memoryRequestGi)
	}
}

func TestInstanceSizeForSnapshotAlignment_UsesSnapshotHugepagesCapability(t *testing.T) {
	size := &ent.InstanceSize{
		RequiresHugepages: false,
		HugepagesSize:     "",
		SpecOverrides:     nil,
	}
	snapshot := map[string]interface{}{
		"requires_hugepages": false,
		"hugepages_size":     "",
		"spec_overrides": map[string]interface{}{
			"spec.template.spec.domain.memory.hugepages.pageSize": "2Mi",
		},
	}
	overrides := resolveInstanceSizeSpecOverrides(size.SpecOverrides, snapshot)

	alignmentSize := instanceSizeForSnapshotAlignment(size, snapshot, overrides)

	if !service.InstanceSizeUsesHugepages(alignmentSize) {
		t.Fatal("expected snapshot spec_overrides hugepages to drive execution alignment")
	}
	if size.HugepagesSize != "" || size.RequiresHugepages {
		t.Fatal("instanceSizeForSnapshotAlignment mutated the live instance size")
	}
}

func TestApplyModifiedSpecOverrides(t *testing.T) {
	spec := &domain.VMSpec{
		Name:     "vm-01",
		CPU:      2,
		MemoryGi: 2,
		DiskGB:   10,
		Image:    "old-image:1",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.cpu.cores": float64(2),
		},
	}

	applyModifiedSpecOverrides(spec, map[string]interface{}{
		"cpu":       4,
		"memory_gi": 4.0,
		"disk_gb":   20,
		"image_source": map[string]interface{}{
			"image": "new-image:2",
		},
		"spec_overrides": map[string]interface{}{
			"spec.template.spec.domain.memory.hugepages.pageSize": "2Mi",
		},
		"spec.template.spec.domain.cpu.cores": float64(4),
	})

	if spec.CPU != 4 {
		t.Fatalf("cpu mismatch: got %.1f", spec.CPU)
	}
	if spec.MemoryGi != 4.0 {
		t.Fatalf("memory mismatch: got %.1f", spec.MemoryGi)
	}
	if spec.DiskGB != 20 {
		t.Fatalf("disk mismatch: got %d", spec.DiskGB)
	}
	if spec.Image != "new-image:2" {
		t.Fatalf("image mismatch: got %q", spec.Image)
	}
	if got := spec.SpecOverrides["spec.template.spec.domain.cpu.cores"]; got != float64(4) {
		t.Fatalf("spec_overrides cpu path mismatch: got %#v", got)
	}
	if got := spec.SpecOverrides["spec.template.spec.domain.memory.hugepages.pageSize"]; got != "2Mi" {
		t.Fatalf("spec_overrides hugepages path mismatch: got %#v", got)
	}
}

func TestApplyModifiedSpecOverrides_ResourceOverrideKeys(t *testing.T) {
	spec := &domain.VMSpec{
		Name:     "vm-01",
		CPU:      2,
		MemoryGi: 2,
		DiskGB:   10,
		Image:    "test-image:1",
	}

	// Simulate modifiedSpec as written by the ticket service when enable_override=true.
	applyModifiedSpecOverrides(spec, map[string]interface{}{
		"enable_override":   true,
		"cpu_limit":         8.0,
		"cpu_request":       4.0,
		"memory_limit_gi":   16.0,
		"memory_request_gi": 8.0,
		"disk_gb":           100,
	})

	if spec.CPU != 8 {
		t.Fatalf("CPU (limit) mismatch: got %.1f, want 8", spec.CPU)
	}
	if spec.CPURequest != 4 {
		t.Fatalf("CPURequest mismatch: got %.1f, want 4", spec.CPURequest)
	}
	if spec.MemoryGi != 16 {
		t.Fatalf("MemoryGi (limit) mismatch: got %.1f, want 16", spec.MemoryGi)
	}
	if spec.MemoryRequestGi != 8 {
		t.Fatalf("MemoryRequestGi mismatch: got %.1f, want 8", spec.MemoryRequestGi)
	}
	if spec.DiskGB != 100 {
		t.Fatalf("DiskGB mismatch: got %d, want 100", spec.DiskGB)
	}
}

func TestApplyModifiedSpecOverrides_CpuLimitTakesPrecedence(t *testing.T) {
	spec := &domain.VMSpec{
		Name:     "vm-01",
		CPU:      2,
		MemoryGi: 2,
		Image:    "test-image:1",
	}

	// Both "cpu" and "cpu_limit" present: cpu_limit should win.
	applyModifiedSpecOverrides(spec, map[string]interface{}{
		"cpu":             4.0,
		"cpu_limit":       8.0,
		"memory_gi":       4.0,
		"memory_limit_gi": 16.0,
	})

	if spec.CPU != 8 {
		t.Fatalf("CPU should use cpu_limit (8) over cpu (4): got %.1f", spec.CPU)
	}
	if spec.MemoryGi != 16 {
		t.Fatalf("MemoryGi should use memory_limit_gi (16) over memory_gi (4): got %.1f", spec.MemoryGi)
	}
}

func TestApplyModifiedSpecOverrides_AlignsGuaranteedRequestsForLimitOnlyOverride(t *testing.T) {
	spec := &domain.VMSpec{
		Name:            "vm-01",
		CPU:             4,
		CPURequest:      4,
		MemoryGi:        8,
		MemoryRequestGi: 8,
		Image:           "test-image:1",
	}

	applyModifiedSpecOverrides(spec, map[string]interface{}{
		"cpu_limit":       8.0,
		"memory_limit_gi": 16.0,
	})

	if spec.CPU != 8 {
		t.Fatalf("CPU mismatch: got %.1f, want 8", spec.CPU)
	}
	if spec.CPURequest != 8 {
		t.Fatalf("CPURequest mismatch: got %.1f, want 8", spec.CPURequest)
	}
	if spec.MemoryGi != 16 {
		t.Fatalf("MemoryGi mismatch: got %.1f, want 16", spec.MemoryGi)
	}
	if spec.MemoryRequestGi != 16 {
		t.Fatalf("MemoryRequestGi mismatch: got %.1f, want 16", spec.MemoryRequestGi)
	}
}

func TestApplyModifiedSpecOverrides_PreservesSharedRequestsForLimitOnlyOverride(t *testing.T) {
	spec := &domain.VMSpec{
		Name:            "vm-01",
		CPU:             4,
		CPURequest:      2,
		MemoryGi:        8,
		MemoryRequestGi: 4,
		Image:           "test-image:1",
	}

	applyModifiedSpecOverrides(spec, map[string]interface{}{
		"cpu_limit":       8.0,
		"memory_limit_gi": 16.0,
	})

	if spec.CPURequest != 2 {
		t.Fatalf("CPURequest mismatch: got %.1f, want 2", spec.CPURequest)
	}
	if spec.MemoryRequestGi != 4 {
		t.Fatalf("MemoryRequestGi mismatch: got %.1f, want 4", spec.MemoryRequestGi)
	}
}

func TestApplyModifiedSpecOverrides_DoesNotAlignMissingRequestsForLimitOnlyOverride(t *testing.T) {
	spec := &domain.VMSpec{
		Name:            "vm-01",
		CPU:             4,
		CPURequest:      0,
		MemoryGi:        8,
		MemoryRequestGi: 0,
		Image:           "test-image:1",
	}

	applyModifiedSpecOverrides(spec, map[string]interface{}{
		"cpu_limit":       8.0,
		"memory_limit_gi": 16.0,
	})

	if spec.CPURequest != 0 {
		t.Fatalf("CPURequest mismatch: got %.1f, want 0", spec.CPURequest)
	}
	if spec.MemoryRequestGi != 0 {
		t.Fatalf("MemoryRequestGi mismatch: got %.1f, want 0", spec.MemoryRequestGi)
	}
}

func TestResolveInstanceSizeSpecOverrides(t *testing.T) {
	base := map[string]interface{}{
		"spec.template.spec.domain.cpu.cores": float64(2),
	}
	snapshot := map[string]interface{}{
		"spec_overrides": map[string]interface{}{
			"spec.template.spec.domain.cpu.cores":                          float64(6),
			"spec.template.spec.domain.memory.hugepages.pageSize":          "2Mi",
			"spec.template.spec.domain.cpu.dedicatedCpuPlacement":          true,
			"spec.template.spec.domain.resources.requests.memory":          "3072Mi",
			"spec.template.spec.domain.resources.limits.memory":            "4096Mi",
			"spec.template.spec.domain.devices.gpus":                       []interface{}{map[string]interface{}{"name": "gpu0", "deviceName": "nvidia.com/A10"}},
			"spec.template.spec.domain.devices.networkInterfaceMultiqueue": true,
		},
	}

	got := resolveInstanceSizeSpecOverrides(base, snapshot)
	if len(got) == 0 {
		t.Fatalf("expected snapshot overrides, got empty map")
	}
	if got["spec.template.spec.domain.cpu.cores"] != float64(6) {
		t.Fatalf("expected snapshot cpu override, got %#v", got["spec.template.spec.domain.cpu.cores"])
	}
	if got["spec.template.spec.domain.memory.hugepages.pageSize"] != "2Mi" {
		t.Fatalf("expected hugepages override, got %#v", got["spec.template.spec.domain.memory.hugepages.pageSize"])
	}

	// Snapshot should override base map for determinism.
	if len(got) == len(base) {
		t.Fatalf("expected snapshot map to replace base overrides")
	}
}

func TestResolveInstanceSizeSpecOverrides_BackwardCompatibleFlatSnapshot(t *testing.T) {
	snapshot := map[string]interface{}{
		"spec.template.spec.domain.cpu.cores": float64(8),
	}
	got := resolveInstanceSizeSpecOverrides(nil, snapshot)
	if got["spec.template.spec.domain.cpu.cores"] != float64(8) {
		t.Fatalf("expected flat snapshot override to be used, got %#v", got["spec.template.spec.domain.cpu.cores"])
	}
}

func TestResolveInstanceSizeDVStorage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		size            *ent.InstanceSize
		snapshot        map[string]interface{}
		wantAccessModes []string
		wantVolumeMode  string
	}{
		{
			name: "snapshot access modes and volume mode win",
			size: &ent.InstanceSize{
				DvAccessModes: []string{"ReadWriteMany"},
				DvVolumeMode:  "Filesystem",
			},
			snapshot: map[string]interface{}{
				"dv_access_modes": []interface{}{" ReadWriteOnce ", "", 42, "ReadOnlyMany"},
				"dv_volume_mode":  " Block ",
			},
			wantAccessModes: []string{"ReadWriteOnce", "ReadOnlyMany"},
			wantVolumeMode:  "Block",
		},
		{
			name: "snapshot volume mode alone wins over instance size storage defaults",
			size: &ent.InstanceSize{
				DvAccessModes: []string{"ReadWriteOnce"},
				DvVolumeMode:  "Filesystem",
			},
			snapshot: map[string]interface{}{
				"dv_volume_mode": "Block",
			},
			wantVolumeMode: "Block",
		},
		{
			name: "instance size storage defaults are cloned and trimmed",
			size: &ent.InstanceSize{
				DvAccessModes: []string{"ReadWriteOnce"},
				DvVolumeMode:  " Filesystem ",
			},
			wantAccessModes: []string{"ReadWriteOnce"},
			wantVolumeMode:  "Filesystem",
		},
		{
			name: "missing instance size and snapshot returns empty storage hints",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotAccessModes, gotVolumeMode := resolveInstanceSizeDVStorage(tc.size, tc.snapshot)
			if !slices.Equal(gotAccessModes, tc.wantAccessModes) {
				t.Fatalf("accessModes = %#v, want %#v", gotAccessModes, tc.wantAccessModes)
			}
			if gotVolumeMode != tc.wantVolumeMode {
				t.Fatalf("volumeMode = %q, want %q", gotVolumeMode, tc.wantVolumeMode)
			}
			if tc.size != nil && len(gotAccessModes) > 0 {
				gotAccessModes[0] = "mutated"
				if slices.Contains(tc.size.DvAccessModes, "mutated") {
					t.Fatal("resolveInstanceSizeDVStorage returned storage access modes aliased to InstanceSize")
				}
			}
		})
	}
}

func TestMapCreatedVMStatusToRow(t *testing.T) {
	testCases := []struct {
		name   string
		vm     *domain.VM
		expect entvm.Status
	}{
		{
			name:   "nil vm defaults to creating",
			vm:     nil,
			expect: entvm.StatusCREATING,
		},
		{
			name:   "running stays running",
			vm:     &domain.VM{Status: domain.VMStatusRunning},
			expect: entvm.StatusRUNNING,
		},
		{
			name:   "failed maps to failed",
			vm:     &domain.VM{Status: domain.VMStatusFailed},
			expect: entvm.StatusFAILED,
		},
		{
			name:   "creating stays creating",
			vm:     &domain.VM{Status: domain.VMStatusCreating},
			expect: entvm.StatusCREATING,
		},
		{
			name:   "pending stays pending",
			vm:     &domain.VM{Status: domain.VMStatusPending},
			expect: entvm.StatusPENDING,
		},
		{
			name:   "unknown stays unknown",
			vm:     &domain.VM{Status: domain.VMStatusUnknown},
			expect: entvm.StatusUNKNOWN,
		},
		{
			name:   "stopping stays stopping",
			vm:     &domain.VM{Status: domain.VMStatusStopping},
			expect: entvm.StatusSTOPPING,
		},
		{
			name:   "stopped maps to stopped",
			vm:     &domain.VM{Status: domain.VMStatusStopped},
			expect: entvm.StatusSTOPPED,
		},
		{
			name:   "deleting maps to deleting",
			vm:     &domain.VM{Status: domain.VMStatusDeleting},
			expect: entvm.StatusDELETING,
		},
		{
			name:   "migrating maps to migrating",
			vm:     &domain.VM{Status: domain.VMStatusMigrating},
			expect: entvm.StatusMIGRATING,
		},
		{
			name:   "paused maps to paused",
			vm:     &domain.VM{Status: domain.VMStatusPaused},
			expect: entvm.StatusPAUSED,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapCreatedVMStatusToRow(tc.vm)
			if got != tc.expect {
				t.Fatalf("status mismatch: got %s want %s", got, tc.expect)
			}
		})
	}
}

func TestLookupStringSliceValue(t *testing.T) {
	t.Parallel()

	t.Run("clones string slice values", func(t *testing.T) {
		source := []string{"ReadWriteOnce"}
		got := lookupStringSliceValue(map[string]interface{}{"dv_access_modes": source}, "missing", "dv_access_modes")
		if !slices.Equal(got, []string{"ReadWriteOnce"}) {
			t.Fatalf("lookupStringSliceValue() = %#v, want ReadWriteOnce", got)
		}
		got[0] = "mutated"
		if source[0] != "ReadWriteOnce" {
			t.Fatal("lookupStringSliceValue returned a slice aliased to the source []string")
		}
	})

	t.Run("filters interface slice values", func(t *testing.T) {
		got := lookupStringSliceValue(map[string]interface{}{
			"dv_access_modes": []interface{}{" ReadWriteOnce ", "", 42, "ReadOnlyMany"},
		}, "dv_access_modes")
		if !slices.Equal(got, []string{"ReadWriteOnce", "ReadOnlyMany"}) {
			t.Fatalf("lookupStringSliceValue() = %#v, want filtered access modes", got)
		}
	})

	t.Run("ignores empty or non-string interface slice values", func(t *testing.T) {
		got := lookupStringSliceValue(map[string]interface{}{
			"dv_access_modes": []interface{}{42, " "},
		}, "dv_access_modes")
		if got != nil {
			t.Fatalf("lookupStringSliceValue() = %#v, want nil", got)
		}
	})
}

func TestToInt(t *testing.T) {
	t.Parallel()

	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name   string
		raw    interface{}
		want   int
		wantOK bool
	}{
		{name: "int8", raw: int8(-8), want: -8, wantOK: true},
		{name: "uint16", raw: uint16(42), want: 42, wantOK: true},
		{name: "uint64 overflow", raw: uint64(maxInt) + 1, wantOK: false},
		{name: "float64 truncates", raw: float64(3.9), want: 3, wantOK: true},
		{name: "trimmed string", raw: " 17 ", want: 17, wantOK: true},
		{name: "invalid string", raw: "not-a-number", wantOK: false},
		{name: "nil", raw: nil, wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toInt(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("toInt(%#v) ok = %v, want %v", tc.raw, ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("toInt(%#v) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestValidateNamespaceClusterEnvironment(t *testing.T) {
	testCases := []struct {
		name         string
		namespaceEnv string
		clusterEnv   string
		expectErr    bool
	}{
		{
			name:         "matching environments",
			namespaceEnv: "test",
			clusterEnv:   "test",
			expectErr:    false,
		},
		{
			name:         "mismatch blocked",
			namespaceEnv: "test",
			clusterEnv:   "prod",
			expectErr:    true,
		},
		{
			name:         "empty blocked",
			namespaceEnv: "",
			clusterEnv:   "prod",
			expectErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNamespaceClusterEnvironment(tc.namespaceEnv, tc.clusterEnv)
			if tc.expectErr && err == nil {
				t.Fatalf("expected error but got nil")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
