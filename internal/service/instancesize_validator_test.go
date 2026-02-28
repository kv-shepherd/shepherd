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
	warnings := DetectSpecOverridesConflicts(4, 8, false, nil, nil)
	if len(warnings) != 0 {
		t.Fatalf("nil overrides should produce no warnings, got: %v", warnings)
	}

	warnings = DetectSpecOverridesConflicts(4, 8, false, nil, map[string]interface{}{
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
	warnings := DetectSpecOverridesConflicts(4, 8, false, nil, overrides)
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
	warnings := DetectSpecOverridesConflicts(4, 8, false, nil, overrides)
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
	warnings := DetectSpecOverridesConflicts(4, 8, true, nil, overrides)
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
	cpuReq := 2.0
	warnings := DetectSpecOverridesConflicts(4, 8, true, &cpuReq, nil)
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
	warnings := DetectSpecOverridesConflicts(4, 8, false, nil, overrides)
	if len(warnings) == 0 {
		t.Fatal("expected memory conflict warning")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "memory_gi") {
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

// TestHasDedicatedCPUInSpecOverrides covers the exported function used by both
// the handler write-path validation and the approval-time bypass guard.
func TestHasDedicatedCPUInSpecOverrides(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
		want      bool
	}{
		{
			name:      "nil overrides",
			overrides: nil,
			want:      false,
		},
		{
			name:      "empty overrides",
			overrides: map[string]interface{}{},
			want:      false,
		},
		{
			name: "bool true — long path",
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,
			},
			want: true,
		},
		{
			name: "bool true — short path",
			overrides: map[string]interface{}{
				"spec.domain.cpu.dedicatedCpuPlacement": true,
			},
			want: true,
		},
		{
			name: "bool false — not dedicated",
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.dedicatedCpuPlacement": false,
			},
			want: false,
		},
		{
			name: "string \"true\" (case-insensitive)",
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.dedicatedCpuPlacement": "True",
			},
			want: true,
		},
		{
			name: "string \"false\" — not dedicated",
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.dedicatedCpuPlacement": "false",
			},
			want: false,
		},
		{
			name: "unrelated path only",
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.cores": 4,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasDedicatedCPUInSpecOverrides(tt.overrides)
			if got != tt.want {
				t.Errorf("HasDedicatedCPUInSpecOverrides() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDetectSpecOverridesConflicts_ReverseDedicated covers the newly added case:
// spec_overrides sets dedicatedCpuPlacement=true but the indexed field is false.
func TestDetectSpecOverridesConflicts_ReverseDedicated(t *testing.T) {
	tests := []struct {
		name          string
		dedicatedCPU  bool
		overrides     map[string]interface{}
		wantWarning   bool
		wantSubstring string
	}{
		{
			name:         "spec=true, indexed=false → warning",
			dedicatedCPU: false,
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,
			},
			wantWarning:   true,
			wantSubstring: "dedicated_cpu field is false",
		},
		{
			name:         "spec=false, indexed=true → warning",
			dedicatedCPU: true,
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.dedicatedCpuPlacement": false,
			},
			wantWarning:   true,
			wantSubstring: "dedicated",
		},
		{
			name:         "spec=true, indexed=true → no conflict warning",
			dedicatedCPU: true,
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.dedicatedCpuPlacement": true,
			},
			wantWarning: false,
		},
		{
			name:         "no dedicated key in spec, indexed=false → no warning",
			dedicatedCPU: false,
			overrides: map[string]interface{}{
				"spec.template.spec.domain.cpu.cores": 4,
			},
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := DetectSpecOverridesConflicts(4, 8192, tt.dedicatedCPU, nil, tt.overrides)
			hasWarning := false
			for _, w := range warnings {
				if tt.wantSubstring != "" && strings.Contains(w, tt.wantSubstring) {
					hasWarning = true
					break
				} else if tt.wantSubstring == "" {
					hasWarning = true
					break
				}
			}
			if tt.wantWarning && !hasWarning {
				t.Errorf("expected warning containing %q, got: %v", tt.wantSubstring, warnings)
			}
			if !tt.wantWarning && len(warnings) > 0 {
				// Filter out unrelated warnings (e.g., overcommit) before asserting
				for _, w := range warnings {
					if strings.Contains(w, "dedicated") {
						t.Errorf("unexpected dedicated_cpu warning: %s", w)
					}
				}
			}
		})
	}
}

// TestHasDedicatedCPUInSpecOverrides_NestedFormat covers the DynamicSchemaForm / Ant Design
// output format where Form.getFieldsValue() returns a nested object tree rather than
// flat dot-notation keys.
//
// The DynamicSchemaForm component registers fields with namePath = path.split('.'),
// which causes Ant Design to store values under nested keys.  JSON.stringify then
// produces {"spec":{"template":{"spec":{"domain":{"cpu":{"dedicatedCpuPlacement":true}}}}}}
// instead of the flat key "spec.template.spec.domain.cpu.dedicatedCpuPlacement".
// Without this nested-format support the constraint bypass described in Finding 1 would
// allow dedicated_cpu=false with a spec override that sets dedicatedCpuPlacement=true.
func TestHasDedicatedCPUInSpecOverrides_NestedFormat(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
		want      bool
	}{
		{
			name: "nested long path — true",
			overrides: map[string]interface{}{
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
			want: true,
		},
		{
			name: "nested long path — false",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"domain": map[string]interface{}{
								"cpu": map[string]interface{}{
									"dedicatedCpuPlacement": false,
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "nested short path — true",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"domain": map[string]interface{}{
						"cpu": map[string]interface{}{
							"dedicatedCpuPlacement": true,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "nested with string 'true'",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"domain": map[string]interface{}{
								"cpu": map[string]interface{}{
									"dedicatedCpuPlacement": "True",
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "nested — unrelated key only",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"domain": map[string]interface{}{
								"cpu": map[string]interface{}{
									"cores": 4,
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "mix: nested true + flat key absent",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"domain": map[string]interface{}{
								"cpu": map[string]interface{}{
									"dedicatedCpuPlacement": true,
									"cores":                 8,
								},
							},
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasDedicatedCPUInSpecOverrides(tt.overrides)
			if got != tt.want {
				t.Errorf("HasDedicatedCPUInSpecOverrides() = %v, want %v (nested format)", got, tt.want)
			}
		})
	}
}

// TestExtractNestedBool covers the internal helper directly as a unit test.
func TestExtractNestedBool(t *testing.T) {
	tests := []struct {
		name    string
		m       map[string]interface{}
		path    []string
		wantVal *bool
	}{
		{
			name:    "nil map",
			m:       nil,
			path:    []string{"a"},
			wantVal: nil,
		},
		{
			name:    "empty path",
			m:       map[string]interface{}{"a": true},
			path:    []string{},
			wantVal: nil,
		},
		{
			name:    "single segment — bool true",
			m:       map[string]interface{}{"x": true},
			path:    []string{"x"},
			wantVal: func() *bool { b := true; return &b }(),
		},
		{
			name:    "single segment — bool false",
			m:       map[string]interface{}{"x": false},
			path:    []string{"x"},
			wantVal: func() *bool { b := false; return &b }(),
		},
		{
			name:    "single segment missing",
			m:       map[string]interface{}{"y": true},
			path:    []string{"x"},
			wantVal: nil,
		},
		{
			name: "two level path",
			m: map[string]interface{}{
				"a": map[string]interface{}{"b": true},
			},
			path:    []string{"a", "b"},
			wantVal: func() *bool { b := true; return &b }(),
		},
		{
			name: "intermediate not a map",
			m: map[string]interface{}{
				"a": "not a map",
			},
			path:    []string{"a", "b"},
			wantVal: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNestedBool(tt.m, tt.path)
			if tt.wantVal == nil {
				if got != nil {
					t.Errorf("expected nil, got %v", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("expected %v, got nil", *tt.wantVal)
				return
			}
			if *got != *tt.wantVal {
				t.Errorf("extractNestedBool() = %v, want %v", *got, *tt.wantVal)
			}
		})
	}
}

// TestSpecOverrideSetsExplicitFalseForDedicatedCPU covers the counterpart to
// HasDedicatedCPUInSpecOverrides: detecting spec_overrides that explicitly set
// dedicatedCpuPlacement=false while dedicated_cpu=true.
//
// Both flat dot-notation keys (DB-stored format) and nested map format
// (DynamicSchemaForm / Ant Design output format) must be detected.
func TestSpecOverrideSetsExplicitFalseForDedicatedCPU(t *testing.T) {
	tests := []struct {
		name         string
		overrides    map[string]interface{}
		wantConflict bool
		wantPathSub  string // expected substring in returned conflict path
	}{
		{name: "nil overrides – no conflict", overrides: nil, wantConflict: false},
		{name: "empty overrides – no conflict", overrides: map[string]interface{}{}, wantConflict: false},
		{
			name:         "flat key=true – not a false conflict",
			overrides:    map[string]interface{}{"spec.template.spec.domain.cpu.dedicatedCpuPlacement": true},
			wantConflict: false,
		},
		{
			name:         "flat long path=false – conflict",
			overrides:    map[string]interface{}{"spec.template.spec.domain.cpu.dedicatedCpuPlacement": false},
			wantConflict: true,
			wantPathSub:  "spec.template.spec.domain.cpu.dedicatedCpuPlacement",
		},
		{
			name:         "flat short path=false – conflict",
			overrides:    map[string]interface{}{"spec.domain.cpu.dedicatedCpuPlacement": false},
			wantConflict: true,
			wantPathSub:  "spec.domain.cpu.dedicatedCpuPlacement",
		},
		{
			name:         "flat key=string \"false\" – conflict",
			overrides:    map[string]interface{}{"spec.template.spec.domain.cpu.dedicatedCpuPlacement": "false"},
			wantConflict: true,
			wantPathSub:  "spec.template.spec.domain.cpu.dedicatedCpuPlacement",
		},
		{
			name:         "flat key=string \"FALSE\" case-insensitive – conflict",
			overrides:    map[string]interface{}{"spec.template.spec.domain.cpu.dedicatedCpuPlacement": "FALSE"},
			wantConflict: true,
			wantPathSub:  "spec.template.spec.domain.cpu.dedicatedCpuPlacement",
		},
		{
			name:         "flat key absent – no conflict",
			overrides:    map[string]interface{}{"spec.template.spec.domain.cpu.cores": 8},
			wantConflict: false,
		},
		{
			name: "nested long path=false – conflict (DynamicSchemaForm format)",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"domain": map[string]interface{}{
								"cpu": map[string]interface{}{"dedicatedCpuPlacement": false},
							},
						},
					},
				},
			},
			wantConflict: true,
			wantPathSub:  "spec.template.spec.domain.cpu.dedicatedCpuPlacement",
		},
		{
			name: "nested long path=true – no conflict",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"domain": map[string]interface{}{
								"cpu": map[string]interface{}{"dedicatedCpuPlacement": true},
							},
						},
					},
				},
			},
			wantConflict: false,
		},
		{
			name: "nested short path=false – conflict",
			overrides: map[string]interface{}{
				"spec": map[string]interface{}{
					"domain": map[string]interface{}{
						"cpu": map[string]interface{}{"dedicatedCpuPlacement": false},
					},
				},
			},
			wantConflict: true,
			wantPathSub:  "spec.domain.cpu.dedicatedCpuPlacement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotConflict := SpecOverrideSetsExplicitFalseForDedicatedCPU(tt.overrides)
			if gotConflict != tt.wantConflict {
				t.Errorf("SpecOverrideSetsExplicitFalseForDedicatedCPU() conflict=%v, want %v (path=%q)",
					gotConflict, tt.wantConflict, gotPath)
				return
			}
			if tt.wantConflict {
				if gotPath == "" {
					t.Errorf("expected non-empty path when conflict=true")
				} else if tt.wantPathSub != "" && !testStringContains(gotPath, tt.wantPathSub) {
					t.Errorf("path %q does not contain expected substring %q", gotPath, tt.wantPathSub)
				}
			} else if gotPath != "" {
				t.Errorf("expected empty path when no conflict, got %q", gotPath)
			}
		})
	}
}

// testStringContains reports whether substr is a substring of s.
func testStringContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
