package provider

import (
	"strings"
	"testing"
)

func renderVMSpecToYAMLForTest(namespace string, spec *VMRenderInput) (string, error) {
	if spec != nil {
		if spec.CPURequest == 0 {
			spec.CPURequest = spec.CPUCores
		}
		if spec.MemoryRequestGi == 0 {
			spec.MemoryRequestGi = spec.MemoryGi
		}
	}
	return RenderVMSpecToYAML(namespace, spec)
}

func TestRenderVMSpecToYAML_RequiresExplicitRequests(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-missing-request",
		CPUCores: 4,
		MemoryGi: 8,
		Image:    "docker.io/kubevirt/centos:7",
	}

	_, err := RenderVMSpecToYAML("test-ns", spec)
	if err == nil {
		t.Fatal("expected missing cpu_request to be rejected")
	}
	if !strings.Contains(err.Error(), "cpu_request must be explicit") {
		t.Fatalf("unexpected error: %v", err)
	}

	spec.CPURequest = 2
	_, err = RenderVMSpecToYAML("test-ns", spec)
	if err == nil {
		t.Fatal("expected missing memory_request_gi to be rejected")
	}
	if !strings.Contains(err.Error(), "memory_request_gi must be explicit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderVMSpecToYAML_ContainerDisk(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-01",
		CPUCores: 4,
		MemoryGi: 8,
		DiskGB:   20,
		Image:    "docker.io/kubevirt/centos:7",
		Labels: map[string]string{
			"env": "test",
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	checks := []string{
		"apiVersion: kubevirt.io/v1",
		"kind: VirtualMachine",
		`name: "vm-01"`,
		`namespace: "test-ns"`,
		"runStrategy: Always",
		"cores: 4",
		`image: "docker.io/kubevirt/centos:7"`,
		"containerDisk:",
		`env: "test"`,
		`cpu: "4"`,
		`memory: "8Gi"`,
	}
	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Errorf("expected YAML to contain %q, got:\n%s", check, yaml)
		}
	}

	if strings.Contains(yaml, "persistentVolumeClaim") {
		t.Errorf("expected no PVC reference for container disk image")
	}
}

func TestRenderVMSpecToYAML_HalfCoreAndHalfGi(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-half",
		CPUCores: 0.5,
		MemoryGi: 0.5,
		Image:    "docker.io/kubevirt/centos:7",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	// 0.5 CPU → "500m"
	if !strings.Contains(yaml, `cpu: "500m"`) {
		t.Errorf("expected cpu=500m for 0.5 cores, got:\n%s", yaml)
	}
	// Topology uses integer cores; 0.5 rounds up to 1 core.
	if !strings.Contains(yaml, "cores: 1") {
		t.Errorf("expected cores=1 for 0.5 cpu topology, got:\n%s", yaml)
	}
	// 0.5 Gi → "512Mi"
	if !strings.Contains(yaml, `memory: "512Mi"`) {
		t.Errorf("expected memory=512Mi for 0.5Gi, got:\n%s", yaml)
	}
	// KubeVirt admission requires spec.template.metadata to be an object, not null.
	if strings.Contains(yaml, "metadata: null") {
		t.Errorf("expected template metadata object, got null:\n%s", yaml)
	}
	// When labels and annotations are absent, metadata renders as "metadata:\n      {}"
	if !strings.Contains(yaml, "metadata:") {
		t.Errorf("expected template metadata present, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_OneAndHalfGi(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-1-5",
		CPUCores: 1.5,
		MemoryGi: 1.5,
		Image:    "docker.io/kubevirt/centos:7",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, `cpu: "1500m"`) {
		t.Errorf("expected cpu=1500m for 1.5 cores, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "cores: 2") {
		t.Errorf("expected cores=2 for 1.5 cpu topology, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, `memory: "1536Mi"`) {
		t.Errorf("expected memory=1536Mi for 1.5Gi, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_ClonePVCImage(t *testing.T) {
	spec := &VMRenderInput{
		Name:         "vm-pvc",
		CPUCores:     2,
		MemoryGi:     4,
		DiskGB:       40,
		Image:        "clone-pvc:my-namespace/my-pvc",
		StorageClass: "fast-sc",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "dataVolumeTemplates:") {
		t.Errorf("expected dataVolumeTemplates for clone-pvc source, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "dataVolume:") {
		t.Errorf("expected dataVolume root volume for clone-pvc source, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, `namespace: "my-namespace"`) || !strings.Contains(yaml, `name: "my-pvc"`) {
		t.Errorf("expected source pvc namespace/name in DataVolumeTemplate, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, `storageClassName: "fast-sc"`) {
		t.Errorf("expected clone DataVolume storageClassName, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, `storage: "40Gi"`) {
		t.Errorf("expected clone DataVolume storage request size, got:\n%s", yaml)
	}
	if strings.Contains(yaml, "persistentVolumeClaim") {
		t.Errorf("expected no direct PVC mount for clone-pvc source")
	}
	if strings.Contains(yaml, "containerDisk") {
		t.Errorf("expected no containerDisk for clone-pvc source")
	}
	if strings.Contains(yaml, "emptyDisk") {
		t.Errorf("expected no extra emptyDisk when DiskGB is used as clone root size")
	}
}

func TestRenderVMSpecToYAML_CDIImageImport(t *testing.T) {
	spec := &VMRenderInput{
		Name:         "vm-import",
		CPUCores:     2,
		MemoryGi:     4,
		DiskGB:       30,
		Image:        "import-image:docker://quay.io/containerdisks/fedora:40",
		StorageClass: "gold-sc",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "dataVolumeTemplates:") {
		t.Errorf("expected dataVolumeTemplates for import-image source, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "registry:") || !strings.Contains(yaml, `url: "docker://quay.io/containerdisks/fedora:40"`) {
		t.Errorf("expected registry import source in DataVolumeTemplate, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, `storage: "30Gi"`) {
		t.Errorf("expected DataVolume storage request size, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, `storageClassName: "gold-sc"`) {
		t.Errorf("expected import DataVolume storageClassName, got:\n%s", yaml)
	}
	if strings.Contains(yaml, "containerDisk") {
		t.Errorf("expected no containerDisk for import-image source")
	}
	if strings.Contains(yaml, "emptyDisk") {
		t.Errorf("expected no extra emptyDisk when DiskGB is used as root DataVolume size")
	}
}

func TestRenderVMSpecToYAML_DirectPVCRejected(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-direct-pvc",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "pvc:my-namespace/my-pvc",
	}

	_, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err == nil {
		t.Fatal("expected error for unsupported direct PVC boot source")
	}
}

func TestRenderVMSpecToYAML_CPUOvercommit(t *testing.T) {
	spec := &VMRenderInput{
		Name:       "vm-overcommit",
		CPUCores:   4,
		CPURequest: 2,
		MemoryGi:   8,
		Image:      "docker.io/kubevirt/centos:7",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, `cpu: "2"`) {
		t.Errorf("expected cpu request=2, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_MemoryOvercommit(t *testing.T) {
	spec := &VMRenderInput{
		Name:            "vm-mem-overcommit",
		CPUCores:        2,
		MemoryGi:        8,
		MemoryRequestGi: 4,
		Image:           "docker.io/kubevirt/centos:7",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, `memory: "4Gi"`) {
		t.Errorf("expected memory request=4Gi, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_WithCloudInit(t *testing.T) {
	spec := &VMRenderInput{
		Name:      "vm-cloudinit",
		CPUCores:  2,
		MemoryGi:  4,
		Image:     "docker.io/kubevirt/centos:7",
		CloudInit: "#cloud-config\nusers:\n  - name: admin",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "cloudinitdisk") {
		t.Errorf("expected cloudinitdisk to be rendered, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "cloudInitNoCloud") {
		t.Errorf("expected cloudInitNoCloud volume to be rendered, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "userData: ") {
		t.Errorf("expected cloud-init userData to be rendered, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_NoDisk(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-nodisk",
		CPUCores: 1,
		MemoryGi: 1,
		DiskGB:   0,
		Image:    "docker.io/kubevirt/centos:7",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if strings.Contains(yaml, "datadisk") {
		t.Errorf("expected no datadisk for DiskGB=0, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-overrides",
		CPUCores: 4,
		MemoryGi: 8,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,
			"spec.template.spec.domain.memory.hugepages.pageSize": "2Mi",
		},
	}

	yamlOut, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yamlOut, "dedicatedCpuPlacement") {
		t.Errorf("expected dedicatedCpuPlacement in rendered YAML, got:\n%s", yamlOut)
	}
	if !strings.Contains(yamlOut, "hugepages") {
		t.Errorf("expected hugepages in rendered YAML, got:\n%s", yamlOut)
	}
	if !strings.Contains(yamlOut, "2Mi") {
		t.Errorf("expected 2Mi pageSize in rendered YAML, got:\n%s", yamlOut)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_RejectsInvalidPath(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-bad-path",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"metadata.labels.foo": "bar", // NOT allowed: must start with spec.*
		},
	}

	_, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err == nil {
		t.Fatalf("expected error for invalid spec_overrides path, got nil")
	}
	if !strings.Contains(err.Error(), "invalid spec_overrides path") {
		t.Errorf("expected 'invalid spec_overrides path' error, got: %v", err)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_RejectsScalarSpecRoot(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-bad-spec-root",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec": "invalid",
		},
	}

	_, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err == nil {
		t.Fatalf("expected error for scalar spec root, got nil")
	}
	if !strings.Contains(err.Error(), "value must be an object") {
		t.Errorf("expected deep-merge object validation error, got: %v", err)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_NestedSpecObjectDeepMerge(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-nested-override",
		CPUCores: 4,
		MemoryGi: 8,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"domain": map[string]interface{}{
							"cpu": map[string]interface{}{
								"dedicatedCpuPlacement": true,
							},
						},
					},
				},
			},
		},
	}

	yamlOut, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	// Ensure nested overrides are merged, not replacing the entire spec object.
	if !strings.Contains(yamlOut, "dedicatedCpuPlacement: true") {
		t.Errorf("expected dedicatedCpuPlacement override in rendered YAML, got:\n%s", yamlOut)
	}
	if !strings.Contains(yamlOut, "runStrategy: Always") {
		t.Errorf("expected template baseline fields to remain after deep merge, got:\n%s", yamlOut)
	}
	if !strings.Contains(yamlOut, "volumes:") {
		t.Errorf("expected template volumes to remain after deep merge, got:\n%s", yamlOut)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_RejectsNonStandardCPU(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-bad-cpu-override",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.resources.requests.cpu": "1300m",
		},
	}

	_, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err == nil {
		t.Fatalf("expected error for non-standard cpu override, got nil")
	}
	if !strings.Contains(err.Error(), "500m increments") {
		t.Errorf("expected cpu half-step validation error, got: %v", err)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_RejectsNonStandardMemory(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-bad-memory-override",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.resources.requests.memory": "1300Mi",
		},
	}

	_, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err == nil {
		t.Fatalf("expected error for non-standard memory override, got nil")
	}
	if !strings.Contains(err.Error(), "512Mi increments") {
		t.Errorf("expected memory half-step validation error, got: %v", err)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_AcceptsStandardResourceSteps(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-good-overrides",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.resources.requests.cpu":    "1500m",
			"spec.template.spec.domain.resources.requests.memory": "1536Mi",
		},
	}

	_, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("expected standard override values to pass, got: %v", err)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_AcceptsNumericMemoryFloat64Mi(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-memory-float64",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			// JSON numbers decode to float64 by default.
			"spec.template.spec.domain.resources.requests.memory": float64(1536), // Mi
		},
	}

	_, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("expected float64 Mi override to pass, got: %v", err)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_EmptyDoesNotChange(t *testing.T) {
	spec := &VMRenderInput{
		Name:          "vm-no-overrides",
		CPUCores:      2,
		MemoryGi:      4,
		Image:         "docker.io/kubevirt/centos:7",
		SpecOverrides: nil,
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if strings.Contains(yaml, "dedicatedCpuPlacement") {
		t.Errorf("unexpected dedicatedCpuPlacement without overrides")
	}
}

func TestRenderVMSpecToYAML_RejectsNonStandardCPU(t *testing.T) {
	tests := []struct {
		name string
		cpu  float64
	}{
		{"0.7 cores", 0.7},
		{"1.2 cores", 1.2},
		{"3.3 cores", 3.3},
		{"0.1 cores", 0.1},
		{"2.8 cores", 2.8},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := &VMRenderInput{Name: "x", CPUCores: tc.cpu, MemoryGi: 1, Image: "img"}
			_, err := renderVMSpecToYAMLForTest("ns", spec)
			if err == nil {
				t.Fatalf("expected error for non-standard CPU %.1f", tc.cpu)
			}
			if !strings.Contains(err.Error(), "0.5-step") {
				t.Errorf("expected '0.5-step' error, got: %v", err)
			}
		})
	}
}

func TestRenderVMSpecToYAML_RejectsNonStandardMemory(t *testing.T) {
	tests := []struct {
		name  string
		memGi float64
	}{
		{"0.7 Gi", 0.7},
		{"1.2 Gi", 1.2},
		{"3.3 Gi", 3.3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := &VMRenderInput{Name: "x", CPUCores: 1, MemoryGi: tc.memGi, Image: "img"}
			_, err := renderVMSpecToYAMLForTest("ns", spec)
			if err == nil {
				t.Fatalf("expected error for non-standard memory %.1fGi", tc.memGi)
			}
			if !strings.Contains(err.Error(), "0.5-step") {
				t.Errorf("expected '0.5-step' error, got: %v", err)
			}
		})
	}
}

func TestRenderVMSpecToYAML_AcceptsStandardSizes(t *testing.T) {
	tests := []struct {
		cpu float64
		mem float64
	}{
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.5},
		{2.0, 2.0},
		{4.0, 8.0},
		{8.0, 16.0},
		{2.5, 3.5},
	}
	for _, tc := range tests {
		spec := &VMRenderInput{Name: "x", CPUCores: tc.cpu, MemoryGi: tc.mem, Image: "img"}
		_, err := renderVMSpecToYAMLForTest("ns", spec)
		if err != nil {
			t.Errorf("unexpected error for standard size cpu=%.1f mem=%.1fGi: %v", tc.cpu, tc.mem, err)
		}
	}
}

func TestRenderVMSpecToYAML_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		spec *VMRenderInput
	}{
		{"nil spec", nil},
		{"no name", &VMRenderInput{CPUCores: 1, MemoryGi: 1, Image: "img"}},
		{"no cpu", &VMRenderInput{Name: "x", MemoryGi: 1, Image: "img"}},
		{"no memory", &VMRenderInput{Name: "x", CPUCores: 1, Image: "img"}},
		{"no image", &VMRenderInput{Name: "x", CPUCores: 1, MemoryGi: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := renderVMSpecToYAMLForTest("ns", tc.spec)
			if err == nil {
				t.Fatalf("expected error for %q", tc.name)
			}
		})
	}
}

func TestRenderVMSpecToYAML_RoundtripExtractName(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "roundtrip-vm",
		CPUCores: 2,
		MemoryGi: 2,
		Image:    "docker.io/kubevirt/centos:7",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	name, err := extractNameFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("extractNameFromYAML returned error: %v", err)
	}
	if name != "roundtrip-vm" {
		t.Fatalf("expected name=roundtrip-vm, got %q", name)
	}
}

func TestRenderVMSpecToYAML_RoundtripWithOverrides(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "override-roundtrip",
		CPUCores: 2,
		MemoryGi: 2,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,
		},
	}

	yamlOut, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	name, err := extractNameFromYAML([]byte(yamlOut))
	if err != nil {
		t.Fatalf("extractNameFromYAML returned error: %v", err)
	}
	if name != "override-roundtrip" {
		t.Fatalf("expected name=override-roundtrip, got %q", name)
	}
}

func TestRenderVMSpecToYAML_ResolvesServiceIDPlaceholderInAffinity(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "affinity-roundtrip",
		CPUCores: 2,
		MemoryGi: 2,
		Image:    "docker.io/kubevirt/centos:7",
		Labels: map[string]string{
			serviceIDLabelKey: "svc-123",
		},
		SpecOverrides: map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"affinity": map[string]interface{}{
							"podAntiAffinity": map[string]interface{}{
								"preferredDuringSchedulingIgnoredDuringExecution": []interface{}{
									map[string]interface{}{
										"weight": float64(100),
										"podAffinityTerm": map[string]interface{}{
											"labelSelector": map[string]interface{}{
												"matchExpressions": []interface{}{
													map[string]interface{}{
														"key":      serviceIDLabelKey,
														"operator": "In",
														"values": []interface{}{
															serviceIDPlaceholderValue,
														},
													},
												},
											},
											"topologyKey": "kubernetes.io/hostname",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	yamlOut, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}
	if !strings.Contains(yamlOut, `shepherd.io/service-id`) {
		t.Fatalf("expected service-id selector in rendered YAML, got:\n%s", yamlOut)
	}
	if !strings.Contains(yamlOut, `- svc-123`) {
		t.Fatalf("expected service-id placeholder resolved in rendered YAML, got:\n%s", yamlOut)
	}
	if strings.Contains(yamlOut, serviceIDPlaceholderValue) {
		t.Fatalf("expected service-id placeholder to be resolved, got:\n%s", yamlOut)
	}
}

func TestIsValidHalfStep(t *testing.T) {
	valid := []float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 4.0, 8.0, 16.0}
	for _, v := range valid {
		if !isValidHalfStep(v) {
			t.Errorf("expected %.1f to be valid half-step", v)
		}
	}

	invalid := []float64{0.1, 0.3, 0.7, 1.2, 1.3, 2.1, 3.3, 0.0, -1.0}
	for _, v := range invalid {
		if isValidHalfStep(v) {
			t.Errorf("expected %.1f to be invalid half-step", v)
		}
	}
}

func TestFormatCPU(t *testing.T) {
	tests := []struct {
		cores    float64
		expected string
	}{
		{0.5, "500m"},
		{1.0, "1"},
		{1.5, "1500m"},
		{2.0, "2"},
		{4.0, "4"},
		{2.5, "2500m"},
	}
	for _, tc := range tests {
		result := formatCPU(tc.cores)
		if result != tc.expected {
			t.Errorf("formatCPU(%.1f) = %q, expected %q", tc.cores, result, tc.expected)
		}
	}
}

func TestCpuCoresForTopology(t *testing.T) {
	tests := []struct {
		cores    float64
		expected int
	}{
		{0.5, 1},
		{1.0, 1},
		{1.5, 2},
		{2.0, 2},
		{2.5, 3},
	}
	for _, tc := range tests {
		got := cpuCoresForTopology(tc.cores)
		if got != tc.expected {
			t.Errorf("cpuCoresForTopology(%.1f) = %d, expected %d", tc.cores, got, tc.expected)
		}
	}
}

func TestFormatGi(t *testing.T) {
	tests := []struct {
		gi       float64
		expected string
	}{
		{0.5, "512Mi"},
		{1.0, "1Gi"},
		{1.5, "1536Mi"},
		{2.0, "2Gi"},
		{4.0, "4Gi"},
		{8.0, "8Gi"},
	}
	for _, tc := range tests {
		result := formatGi(tc.gi)
		if result != tc.expected {
			t.Errorf("formatGi(%.1f) = %q, expected %q", tc.gi, result, tc.expected)
		}
	}
}

// --- Production-grade feature tests (ADR-0018 Hybrid Model) ---
// These tests validate that VM behavior configs are correctly injected via
// spec_overrides deep-merge, not via template conditional blocks.

func TestRenderVMSpecToYAML_DVAccessModesAndVolumeMode(t *testing.T) {
	// DV access modes use explicit fields (structural DV format change).
	spec := &VMRenderInput{
		Name:          "vm-dv-modes",
		CPUCores:      2,
		MemoryGi:      4,
		DiskGB:        60,
		Image:         "clone-pvc:vm-muban/openeuler2203-image",
		StorageClass:  "rook-ceph-block",
		DVAccessModes: []string{"ReadWriteMany"},
		DVVolumeMode:  "Block",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	checks := []string{
		"accessModes:",
		"- ReadWriteMany",
		"volumeMode: Block",
		`storageClassName: "rook-ceph-block"`,
		`storage: "60Gi"`,
	}
	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Errorf("expected YAML to contain %q, got:\n%s", check, yaml)
		}
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_CPUModel(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-cpu-model",
		CPUCores: 4,
		MemoryGi: 8,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.cpu.model": "host-passthrough",
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "model: host-passthrough") {
		t.Errorf("expected cpu model host-passthrough via spec_overrides, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_BridgeNetwork(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-bridge",
		CPUCores: 4,
		MemoryGi: 8,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.devices.interfaces":                 []interface{}{map[string]interface{}{"bridge": map[string]interface{}{}, "model": "virtio", "name": "default"}},
			"spec.template.spec.domain.devices.networkInterfaceMultiqueue": true,
			"spec.template.spec.networks":                                  []interface{}{map[string]interface{}{"name": "default", "pod": map[string]interface{}{}}},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	checks := []string{
		"interfaces:",
		"model: virtio",
		"name: default",
		"networkInterfaceMultiqueue: true",
		"networks:",
	}
	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Errorf("expected YAML to contain %q, got:\n%s", check, yaml)
		}
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_EvictionStrategy(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-eviction",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.evictionStrategy": "LiveMigrate",
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "evictionStrategy: LiveMigrate") {
		t.Errorf("expected evictionStrategy via spec_overrides, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_LivenessProbe(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-liveness",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.livenessProbe": map[string]interface{}{
				"failureThreshold":    int64(3),
				"guestAgentPing":      map[string]interface{}{},
				"initialDelaySeconds": int64(120),
				"periodSeconds":       int64(20),
				"timeoutSeconds":      int64(5),
			},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	checks := []string{
		"livenessProbe:",
		"guestAgentPing:",
		"initialDelaySeconds:",
	}
	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Errorf("expected YAML to contain %q, got:\n%s", check, yaml)
		}
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_DeviceOptimizations(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-optimized",
		CPUCores: 4,
		MemoryGi: 8,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.devices.autoattachGraphicsDevice": false,
			"spec.template.spec.domain.devices.autoattachMemBalloon":     false,
			"spec.template.spec.domain.devices.autoattachSerialConsole":  true,
			"spec.template.spec.domain.devices.autoattachVSOCK":          true,
			"spec.template.spec.domain.devices.blockMultiQueue":          true,
			"spec.template.spec.domain.devices.rng":                      map[string]interface{}{},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	checks := []string{
		"autoattachGraphicsDevice: false",
		"autoattachMemBalloon: false",
		"autoattachSerialConsole: true",
		"autoattachVSOCK: true",
		"blockMultiQueue: true",
		"rng:",
	}
	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Errorf("expected YAML to contain %q, got:\n%s", check, yaml)
		}
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_TemplateAnnotations(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-annotations",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.metadata.annotations": map[string]interface{}{
				"kubevirt.io/allow-pod-bridge-network-live-migration": "true",
				"ovn.kubernetes.io/allow_live_migration":              "true",
			},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	checks := []string{
		"annotations:",
		"kubevirt.io/allow-pod-bridge-network-live-migration",
		"ovn.kubernetes.io/allow_live_migration",
	}
	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Errorf("expected YAML to contain %q, got:\n%s", check, yaml)
		}
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_NestedNodeSelectorPreservesLiteralDots(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-node-selector",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"nodeSelector": map[string]interface{}{
							"kubevirt.io/ksm-enabled": "true",
						},
					},
				},
			},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "nodeSelector:") {
		t.Fatalf("expected nodeSelector in rendered YAML, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "kubevirt.io/ksm-enabled: \"true\"") {
		t.Fatalf("expected literal dotted nodeSelector key in rendered YAML, got:\n%s", yaml)
	}
	if strings.Contains(yaml, "kubevirt:\n") {
		t.Fatalf("expected dotted nodeSelector key to stay literal, got nested map:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_SourceStyleDisksAndNetworks(t *testing.T) {
	spec := &VMRenderInput{
		Name:      "vm-source-style",
		CPUCores:  4,
		MemoryGi:  8,
		Image:     "clone-pvc:vm-muban/openeuler2203-image",
		CloudInit: "#cloud-config\nusers:\n  - default",
		SpecOverrides: map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"domain": map[string]interface{}{
							"devices": map[string]interface{}{
								"disks": []interface{}{
									map[string]interface{}{
										"name": "rootfs",
										"disk": map[string]interface{}{"bus": "virtio"},
									},
									map[string]interface{}{
										"name": "cloudinitdisk",
										"disk": map[string]interface{}{"bus": "virtio"},
									},
								},
								"interfaces": []interface{}{
									map[string]interface{}{
										"bridge": map[string]interface{}{},
										"name":   "default",
										"model":  "virtio",
									},
								},
							},
						},
						"networks": []interface{}{
							map[string]interface{}{
								"name": "default",
								"pod":  map[string]interface{}{},
							},
						},
					},
				},
			},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	checks := []string{
		"name: rootfs",
		"name: cloudinitdisk",
		"interfaces:",
		"networks:",
		"pod: {}",
		"dataVolume:",
	}
	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Fatalf("expected rendered YAML to contain %q, got:\n%s", check, yaml)
		}
	}
	if strings.Contains(yaml, "- name: rootdisk") {
		t.Fatalf("expected rendered YAML not to use legacy rootdisk volume name, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_TerminationGracePeriod(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-term-grace",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.terminationGracePeriodSeconds": int64(0),
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "terminationGracePeriodSeconds:") {
		t.Errorf("expected terminationGracePeriodSeconds via spec_overrides, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_GuestMemory(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-guest-mem",
		CPUCores: 4,
		MemoryGi: 8,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.memory.guest": "8Gi",
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "guest: 8Gi") {
		t.Errorf("expected guest memory via spec_overrides, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_SpecOverrides_Architecture(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-arch",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.architecture": "amd64",
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "architecture: amd64") {
		t.Errorf("expected architecture via spec_overrides, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_NormalizesDefaultInterfaceAndNetworkWhenEmptyObjectsWerePruned(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-normalize-net",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.devices.interfaces": []interface{}{
				map[string]interface{}{"model": "virtio", "name": "default"},
			},
			"spec.template.spec.networks": []interface{}{
				map[string]interface{}{"name": "default"},
			},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	for _, check := range []string{"bridge: {}", "pod: {}", "name: default"} {
		if !strings.Contains(yaml, check) {
			t.Fatalf("expected normalized YAML to contain %q, got:\n%s", check, yaml)
		}
	}
}

func TestRenderVMSpecToYAML_RemovesManagedCloudInitDiskWhenTemplateHasNoCloudInit(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-cloudinit-fallback",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "clone-pvc:vm-muban/openeuler2203-image",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.devices.disks": []interface{}{
				map[string]interface{}{
					"name": "rootfs",
					"disk": map[string]interface{}{"bus": "virtio"},
				},
				map[string]interface{}{
					"name": "cloudinitdisk",
					"disk": map[string]interface{}{"bus": "virtio"},
				},
			},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if strings.Contains(yaml, "name: cloudinitdisk") {
		t.Fatalf("expected renderer to remove dangling cloudinitdisk when template has no cloud-init, got:\n%s", yaml)
	}
	if strings.Contains(yaml, "cloudInitNoCloud:") {
		t.Fatalf("expected no cloudInitNoCloud volume when template has no cloud-init, got:\n%s", yaml)
	}
}

func TestRenderVMSpecToYAML_RestoresBaseCloudInitVolumeWhenOverridesDropIt(t *testing.T) {
	spec := &VMRenderInput{
		Name:      "vm-cloudinit-restore",
		CPUCores:  2,
		MemoryGi:  4,
		Image:     "clone-pvc:vm-muban/openeuler2203-image",
		CloudInit: "#cloud-config\nusers:\n  - name: admin",
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.domain.devices.disks": []interface{}{
				map[string]interface{}{
					"name": "rootfs",
					"disk": map[string]interface{}{"bus": "virtio"},
				},
				map[string]interface{}{
					"name": "cloudinitdisk",
					"disk": map[string]interface{}{"bus": "virtio"},
				},
			},
			"spec.template.spec.volumes": []interface{}{
				map[string]interface{}{
					"name":       "rootfs",
					"dataVolume": map[string]interface{}{"name": "vm-cloudinit-restore-rootfs"},
				},
			},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	for _, check := range []string{"name: cloudinitdisk", "cloudInitNoCloud:", "#cloud-config", "- name: admin"} {
		if !strings.Contains(yaml, check) {
			t.Fatalf("expected rendered YAML to contain %q, got:\n%s", check, yaml)
		}
	}
}

func TestRenderVMSpecToYAML_NoNetworkByDefault(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-no-net",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "docker.io/kubevirt/centos:7",
	}

	yaml, err := renderVMSpecToYAMLForTest("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if strings.Contains(yaml, "interfaces:") {
		t.Errorf("expected no interfaces when no spec_overrides, got:\n%s", yaml)
	}
	if strings.Contains(yaml, "networks:") {
		t.Errorf("expected no networks when no spec_overrides, got:\n%s", yaml)
	}
}

// TestRenderVMSpecToYAML_ProductionGradeFullSpec tests a full production-grade
// VM spec using spec_overrides (ADR-0018 Hybrid Model).
func TestRenderVMSpecToYAML_ProductionGradeFullSpec(t *testing.T) {
	spec := &VMRenderInput{
		Name:            "prod-vm-01",
		CPUCores:        4,
		CPURequest:      2,
		MemoryGi:        8,
		MemoryRequestGi: 4,
		DiskGB:          60,
		Image:           "clone-pvc:vm-muban/openeuler2203-image",
		StorageClass:    "rook-ceph-block",
		CloudInit:       "#cloud-config\nhostname: prod-vm\n",

		// DV storage mode (explicit fields).
		DVAccessModes: []string{"ReadWriteMany"},
		DVVolumeMode:  "Block",

		// All VM behavior configs via spec_overrides (ADR-0018).
		SpecOverrides: map[string]interface{}{
			"spec.template.spec.architecture":                            "amd64",
			"spec.template.spec.domain.cpu.model":                        "host-passthrough",
			"spec.template.spec.domain.ioThreadsPolicy":                  "auto",
			"spec.template.spec.domain.memory.guest":                     "8Gi",
			"spec.template.spec.evictionStrategy":                        "LiveMigrate",
			"spec.template.spec.terminationGracePeriodSeconds":           int64(0),
			"spec.template.spec.domain.devices.autoattachGraphicsDevice": false,
			"spec.template.spec.domain.devices.autoattachMemBalloon":     false,
			"spec.template.spec.domain.devices.blockMultiQueue":          true,
			"spec.template.spec.domain.devices.rng":                      map[string]interface{}{},
			"spec.template.spec.domain.devices.interfaces": []interface{}{
				map[string]interface{}{"bridge": map[string]interface{}{}, "model": "virtio", "name": "default"},
			},
			"spec.template.spec.domain.devices.networkInterfaceMultiqueue": true,
			"spec.template.spec.networks": []interface{}{
				map[string]interface{}{"name": "default", "pod": map[string]interface{}{}},
			},
			"spec.template.spec.livenessProbe": map[string]interface{}{
				"failureThreshold": int64(3), "guestAgentPing": map[string]interface{}{},
				"initialDelaySeconds": int64(120), "periodSeconds": int64(20), "timeoutSeconds": int64(5),
			},
			"spec.template.metadata.annotations": map[string]interface{}{
				"kubevirt.io/allow-pod-bridge-network-live-migration": "true",
			},
		},
	}

	yaml, err := renderVMSpecToYAMLForTest("prod-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	// Verify all production-grade fields are present.
	checks := []string{
		"accessModes:",
		"- ReadWriteMany",
		"volumeMode: Block",
		"architecture: amd64",
		"model: host-passthrough",
		"ioThreadsPolicy: auto",
		"evictionStrategy: LiveMigrate",
		"livenessProbe:",
		"guestAgentPing:",
		"terminationGracePeriodSeconds:",
		"autoattachGraphicsDevice: false",
		"autoattachMemBalloon: false",
		"blockMultiQueue: true",
		"rng:",
		"interfaces:",
		"networkInterfaceMultiqueue: true",
		"networks:",
		"annotations:",
		"kubevirt.io/allow-pod-bridge-network-live-migration",
		"cloudInitNoCloud:",
		`cpu: "2"`,    // CPU overcommit request
		`memory: 4Gi`, // Memory overcommit request
	}
	for _, check := range checks {
		if !strings.Contains(yaml, check) {
			t.Errorf("expected production YAML to contain %q, got:\n%s", check, yaml)
		}
	}
}
