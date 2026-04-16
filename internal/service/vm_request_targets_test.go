package service

import (
	"strings"
	"testing"
)

func TestValidateVMRequestTargets(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		targets VMRequestTargets
		wantErr string
	}{
		{
			name: "accepts empty overrides",
		},
		{
			name: "rejects cpu below minimum",
			targets: VMRequestTargets{
				TargetCPUCores: float64Ptr(0.25),
			},
			wantErr: "target_cpu_cores must be >= 0.5",
		},
		{
			name: "rejects cpu non half step",
			targets: VMRequestTargets{
				TargetCPUCores: float64Ptr(0.75),
			},
			wantErr: "target_cpu_cores must use 0.5-step values",
		},
		{
			name: "rejects memory below minimum",
			targets: VMRequestTargets{
				TargetMemoryGi: float64Ptr(0.25),
			},
			wantErr: "target_memory_gi must be >= 0.5",
		},
		{
			name: "rejects disk below minimum",
			targets: VMRequestTargets{
				TargetDiskGB: intPtr(0),
			},
			wantErr: "target_disk_gb must be >= 1",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateVMRequestTargets(tc.targets)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("ValidateVMRequestTargets() unexpected error = %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("ValidateVMRequestTargets() error = nil, want substring %q", tc.wantErr)
			case tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("ValidateVMRequestTargets() error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestResolveVMRequestTargets(t *testing.T) {
	t.Parallel()

	resolved := ResolveVMRequestTargets(
		4,
		3,
		8,
		6,
		80,
		VMRequestTargets{
			TargetCPUCores: float64Ptr(2),
			TargetMemoryGi: float64Ptr(4),
			TargetDiskGB:   intPtr(120),
		},
	)

	if resolved.CPULimit != 2 {
		t.Fatalf("CPULimit = %v, want 2", resolved.CPULimit)
	}
	if resolved.CPURequest != 2 {
		t.Fatalf("CPURequest = %v, want 2", resolved.CPURequest)
	}
	if !resolved.HasCustomCPULimit || !resolved.AdjustedCPURequest {
		t.Fatalf("expected CPU override and request adjustment, got %+v", resolved)
	}
	if resolved.MemoryLimitGi != 4 {
		t.Fatalf("MemoryLimitGi = %v, want 4", resolved.MemoryLimitGi)
	}
	if resolved.MemoryRequestGi != 4 {
		t.Fatalf("MemoryRequestGi = %v, want 4", resolved.MemoryRequestGi)
	}
	if !resolved.HasCustomMemoryGi || !resolved.AdjustedMemoryGiReq {
		t.Fatalf("expected memory override and request adjustment, got %+v", resolved)
	}
	if resolved.DiskGB != 120 {
		t.Fatalf("DiskGB = %d, want 120", resolved.DiskGB)
	}
	if !resolved.HasCustomDiskGB {
		t.Fatalf("expected custom disk flag, got %+v", resolved)
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}
