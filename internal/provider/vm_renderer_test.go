package provider

import (
	"strings"
	"testing"
)

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

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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
}

func TestRenderVMSpecToYAML_OneAndHalfGi(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-1-5",
		CPUCores: 1.5,
		MemoryGi: 1.5,
		Image:    "docker.io/kubevirt/centos:7",
	}

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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

func TestRenderVMSpecToYAML_PVCImage(t *testing.T) {
	spec := &VMRenderInput{
		Name:     "vm-pvc",
		CPUCores: 2,
		MemoryGi: 4,
		Image:    "pvc:my-namespace/my-pvc",
	}

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
	if err != nil {
		t.Fatalf("RenderVMSpecToYAML returned error: %v", err)
	}

	if !strings.Contains(yaml, "persistentVolumeClaim") {
		t.Errorf("expected PVC reference, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, `claimName: "my-pvc"`) {
		t.Errorf("expected claimName=my-pvc, got:\n%s", yaml)
	}
	if strings.Contains(yaml, "containerDisk") {
		t.Errorf("expected no containerDisk for PVC image")
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

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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

	yamlOut, err := RenderVMSpecToYAML("test-ns", spec)
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

	_, err := RenderVMSpecToYAML("test-ns", spec)
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

	_, err := RenderVMSpecToYAML("test-ns", spec)
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

	yamlOut, err := RenderVMSpecToYAML("test-ns", spec)
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

	_, err := RenderVMSpecToYAML("test-ns", spec)
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

	_, err := RenderVMSpecToYAML("test-ns", spec)
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

	_, err := RenderVMSpecToYAML("test-ns", spec)
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

	_, err := RenderVMSpecToYAML("test-ns", spec)
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

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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
			_, err := RenderVMSpecToYAML("ns", spec)
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
			_, err := RenderVMSpecToYAML("ns", spec)
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
		_, err := RenderVMSpecToYAML("ns", spec)
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
			_, err := RenderVMSpecToYAML("ns", tc.spec)
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

	yaml, err := RenderVMSpecToYAML("test-ns", spec)
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

	yamlOut, err := RenderVMSpecToYAML("test-ns", spec)
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
