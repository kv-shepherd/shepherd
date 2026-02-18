package jobs

import (
	"testing"

	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
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
			expectImage: "pvc:centos-base",
		},
		{
			name: "volumes containerDisk fallback",
			spec: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "rootdisk",
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

func TestApplyModifiedSpecOverrides(t *testing.T) {
	spec := &domain.VMSpec{
		Name:     "vm-01",
		CPU:      2,
		MemoryMB: 2048,
		DiskGB:   10,
		Image:    "old-image:1",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.cpu.cores": float64(2),
		},
	}

	applyModifiedSpecOverrides(spec, map[string]interface{}{
		"cpu":       4,
		"memory_mb": "4096",
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
		t.Fatalf("cpu mismatch: got %d", spec.CPU)
	}
	if spec.MemoryMB != 4096 {
		t.Fatalf("memory mismatch: got %d", spec.MemoryMB)
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

func TestMapCreatedVMStatusToRow(t *testing.T) {
	testCases := []struct {
		name   string
		vm     *domain.VM
		expect entvm.Status
	}{
		{
			name:   "nil vm defaults to running",
			vm:     nil,
			expect: entvm.StatusRUNNING,
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
			name:   "creating promoted to running",
			vm:     &domain.VM{Status: domain.VMStatusCreating},
			expect: entvm.StatusRUNNING,
		},
		{
			name:   "pending promoted to running",
			vm:     &domain.VM{Status: domain.VMStatusPending},
			expect: entvm.StatusRUNNING,
		},
		{
			name:   "unknown promoted to running",
			vm:     &domain.VM{Status: domain.VMStatusUnknown},
			expect: entvm.StatusRUNNING,
		},
		{
			name:   "stopping maps to stopping",
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
