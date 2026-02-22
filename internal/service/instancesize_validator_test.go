package service

import (
	"strings"
	"testing"
)

func TestValidateSpecOverrides_NilAndEmpty(t *testing.T) {
	if err := ValidateSpecOverrides(nil); err != nil {
		t.Fatalf("nil overrides should pass: %v", err)
	}
	if err := ValidateSpecOverrides(map[string]interface{}{}); err != nil {
		t.Fatalf("empty overrides should pass: %v", err)
	}
}

func TestValidateSpecOverrides_ValidPaths(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
	}{
		{
			name: "spec prefix with dot",
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,
			},
		},
		{
			name: "spec root key (nested object)",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
		},
		{
			name: "multiple valid paths",
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.cores":                          4,
				"spec.template.spec.domain.memory.hugepages.pageSize":          "2Mi",
				"spec.template.spec.domain.devices.networkInterfaceMultiqueue": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSpecOverrides(tt.overrides); err != nil {
				t.Errorf("expected valid overrides to pass, got: %v", err)
			}
		})
	}
}

func TestValidateSpecOverrides_InvalidPaths(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
	}{
		{
			name: "plain key without spec prefix",
			overrides: map[string]interface{}{
				"domain.cpu.cores": 4,
			},
		},
		{
			name: "metadata path",
			overrides: map[string]interface{}{
				"metadata.labels.app": "test",
			},
		},
		{
			name: "status path",
			overrides: map[string]interface{}{
				"status.ready": true,
			},
		},
		{
			name: "mixed valid and invalid",
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.cores": 4,
				"template.spec.domain.memory":         "4Gi",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpecOverrides(tt.overrides)
			if err == nil {
				t.Error("expected validation error for invalid spec_overrides path")
				return
			}
			if !strings.Contains(err.Error(), "spec.") {
				t.Errorf("expected error to mention 'spec.' prefix requirement, got: %v", err)
			}
		})
	}
}

func TestDetectSpecOverridesConflicts_NoConflicts(t *testing.T) {
	warnings := DetectSpecOverridesConflicts(4, 8192, false, nil, nil)
	if len(warnings) != 0 {
		t.Fatalf("nil overrides should produce no warnings, got: %v", warnings)
	}

	warnings = DetectSpecOverridesConflicts(4, 8192, false, nil, map[string]interface{}{
		"spec.template.spec.domain.devices.networkInterfaceMultiqueue": true,
	})
	if len(warnings) != 0 {
		t.Fatalf("non-conflicting overrides should produce no warnings, got: %v", warnings)
	}
}

func TestDetectSpecOverridesConflicts_CPUConflict(t *testing.T) {
	overrides := map[string]interface{}{
		"spec.template.spec.domain.cpu.cores": 8,
	}
	warnings := DetectSpecOverridesConflicts(4, 8192, false, nil, overrides)
	if len(warnings) == 0 {
		t.Fatal("expected CPU conflict warning")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "cpu_cores=4") && strings.Contains(w, "8") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning mentioning cpu_cores conflict, got: %v", warnings)
	}
}

func TestDetectSpecOverridesConflicts_CPUMatch(t *testing.T) {
	overrides := map[string]interface{}{
		"spec.template.spec.domain.cpu.cores": 4,
	}
	warnings := DetectSpecOverridesConflicts(4, 8192, false, nil, overrides)
	for _, w := range warnings {
		if strings.Contains(w, "cpu_cores") {
			t.Errorf("matching CPU should not produce conflict warning, got: %s", w)
		}
	}
}

func TestDetectSpecOverridesConflicts_DedicatedCPUInconsistency(t *testing.T) {
	overrides := map[string]interface{}{
		"spec.template.spec.domain.cpu.dedicatedCpuPlacement": false,
	}
	warnings := DetectSpecOverridesConflicts(4, 8192, true, nil, overrides)
	if len(warnings) == 0 {
		t.Fatal("expected warning for dedicated_cpu inconsistency")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "dedicated") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected dedicated_cpu inconsistency warning, got: %v", warnings)
	}
}

func TestDetectSpecOverridesConflicts_DedicatedCPUOvercommit(t *testing.T) {
	cpuReq := 2
	warnings := DetectSpecOverridesConflicts(4, 8192, true, &cpuReq, nil)
	if len(warnings) == 0 {
		t.Fatal("expected warning for dedicated_cpu + overcommit")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "Guaranteed QoS") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected QoS warning, got: %v", warnings)
	}
}

func TestDetectSpecOverridesConflicts_MemoryConflict(t *testing.T) {
	overrides := map[string]interface{}{
		"spec.template.spec.domain.resources.requests.memory": "16Gi",
	}
	warnings := DetectSpecOverridesConflicts(4, 8192, false, nil, overrides)
	if len(warnings) == 0 {
		t.Fatal("expected memory conflict warning")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "memory_mb") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected memory conflict warning, got: %v", warnings)
	}
}

func TestToIntSafe(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
		ok       bool
	}{
		{"int", 42, 42, true},
		{"int64", int64(100), 100, true},
		{"float64 exact", float64(8), 8, true},
		{"float64 non-exact", float64(8.5), 0, false},
		{"string", "42", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := toIntSafe(tt.input)
			if ok != tt.ok {
				t.Errorf("toIntSafe(%v) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && v != tt.expected {
				t.Errorf("toIntSafe(%v) = %d, want %d", tt.input, v, tt.expected)
			}
		})
	}
}
