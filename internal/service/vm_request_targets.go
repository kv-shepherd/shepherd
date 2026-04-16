package service

import "fmt"

type VMRequestTargets struct {
	TargetCPUCores *float64
	TargetMemoryGi *float64
	TargetDiskGB   *int
}

type ResolvedVMRequestTargets struct {
	CPULimit            float64
	MemoryLimitGi       float64
	DiskGB              int
	CPURequest          float64
	MemoryRequestGi     float64
	HasCustomCPULimit   bool
	HasCustomMemoryGi   bool
	HasCustomDiskGB     bool
	AdjustedCPURequest  bool
	AdjustedMemoryGiReq bool
}

func ValidateVMRequestTargets(targets VMRequestTargets) error {
	if targets.TargetCPUCores != nil {
		if *targets.TargetCPUCores < 0.5 {
			return fmt.Errorf("target_cpu_cores must be >= 0.5")
		}
		if !IsHalfStep(*targets.TargetCPUCores) {
			return fmt.Errorf("target_cpu_cores must use 0.5-step values (0.5, 1.0, 1.5, ...)")
		}
	}
	if targets.TargetMemoryGi != nil {
		if *targets.TargetMemoryGi < 0.5 {
			return fmt.Errorf("target_memory_gi must be >= 0.5")
		}
		if !IsHalfStep(*targets.TargetMemoryGi) {
			return fmt.Errorf("target_memory_gi must use 0.5-step values (0.5, 1.0, 1.5, ...)")
		}
	}
	if targets.TargetDiskGB != nil && *targets.TargetDiskGB < 1 {
		return fmt.Errorf("target_disk_gb must be >= 1")
	}
	return nil
}

func ResolveVMRequestTargets(
	defaultCPULimit float64,
	defaultCPURequest float64,
	defaultMemoryLimitGi float64,
	defaultMemoryRequestGi float64,
	defaultDiskGB int,
	targets VMRequestTargets,
) ResolvedVMRequestTargets {
	resolved := ResolvedVMRequestTargets{
		CPULimit:        defaultCPULimit,
		MemoryLimitGi:   defaultMemoryLimitGi,
		DiskGB:          defaultDiskGB,
		CPURequest:      defaultCPURequest,
		MemoryRequestGi: defaultMemoryRequestGi,
	}

	if targets.TargetCPUCores != nil && *targets.TargetCPUCores > 0 {
		resolved.CPULimit = *targets.TargetCPUCores
		resolved.HasCustomCPULimit = true
	}
	if targets.TargetMemoryGi != nil && *targets.TargetMemoryGi > 0 {
		resolved.MemoryLimitGi = *targets.TargetMemoryGi
		resolved.HasCustomMemoryGi = true
	}
	if targets.TargetDiskGB != nil && *targets.TargetDiskGB > 0 {
		resolved.DiskGB = *targets.TargetDiskGB
		resolved.HasCustomDiskGB = true
	}

	if resolved.CPULimit > 0 && resolved.CPURequest > resolved.CPULimit {
		resolved.CPURequest = resolved.CPULimit
		resolved.AdjustedCPURequest = true
	}
	if resolved.MemoryLimitGi > 0 && resolved.MemoryRequestGi > resolved.MemoryLimitGi {
		resolved.MemoryRequestGi = resolved.MemoryLimitGi
		resolved.AdjustedMemoryGiReq = true
	}

	return resolved
}
