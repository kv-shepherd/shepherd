// Package domain provides example domain entities for KubeVirt Shepherd.
//
// This file defines the InstanceSize entity per ADR-0018.
// InstanceSize abstracts KubeVirt VM resource configuration.
//
// Reference: docs/adr/ADR-0018-instance-size-abstraction.md

package domain

import (
	"errors"
	"time"
)

// InstanceSize represents a predefined VM resource configuration.
// Admin-managed entity that defines CPU, Memory, and optional overcommit settings.
//
// Key Design Decisions (ADR-0018):
// - Name is globally unique (index.Fields("name").Unique())
// - spec_overrides stores KubeVirt-specific configuration as JSONB
// - Immutability: VMs snapshot InstanceSize at approval time (changes don't affect existing VMs)
type InstanceSize struct {
	ID          string `json:"id"`
	Name        string `json:"name"` // Globally unique, e.g., "small", "medium-gpu"
	Description string `json:"description,omitempty"`

	// Core scheduling fields (indexed for query performance)
	CPU         int `json:"cpu"`       // Logical CPUs
	MemoryMB    int `json:"memory_mb"` // Memory in MB
	GPUCount    int `json:"gpu_count,omitempty"`
	HugepagesGB int `json:"hugepages_gb,omitempty"`

	// Overcommit settings (ADR-0018 §5)
	CPUOvercommit float64 `json:"cpu_overcommit,omitempty"` // e.g., 0.5 = 50% of limit
	MemOvercommit float64 `json:"mem_overcommit,omitempty"` // e.g., 0.8 = 80% of limit

	// KubeVirt-specific configuration (JSONB in backend)
	// Contains: dedicatedCpuPlacement, isolateEmulatorThread, ioThreadsPolicy, etc.
	SpecOverrides map[string]interface{} `json:"spec_overrides,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OvercommitConfig defines overcommit settings that can be used
// in both InstanceSize and ApprovalTicket (admin override).
type OvercommitConfig struct {
	// CPURatio: request = limit * CPURatio (e.g., 0.5 = 50%)
	// 1.0 = Guaranteed QoS (no overcommit)
	// <1.0 = Burstable QoS
	CPURatio float64 `json:"cpu_ratio,omitempty"`

	// MemRatio: request = limit * MemRatio (e.g., 0.8 = 80%)
	// 1.0 = Guaranteed QoS (no overcommit)
	// <1.0 = Burstable QoS
	MemRatio float64 `json:"mem_ratio,omitempty"`
}

// IsGuaranteedQoS returns true if this config results in Guaranteed QoS.
// Guaranteed QoS requires: request == limit (ratio = 1.0 for both CPU and Memory)
func (c *OvercommitConfig) IsGuaranteedQoS() bool {
	return c.CPURatio >= 1.0 && c.MemRatio >= 1.0
}

// ValidateWithDedicatedCPU checks if overcommit is compatible with dedicated CPU.
// Per KubeVirt documentation, dedicatedCpuPlacement requires Guaranteed QoS.
//
// Returns error if:
// - Dedicated CPU is enabled AND overcommit < 1.0 (Burstable QoS)
func (c *OvercommitConfig) ValidateWithDedicatedCPU(dedicatedCPU bool) error {
	if dedicatedCPU && !c.IsGuaranteedQoS() {
		return ErrDedicatedCPURequiresGuaranteedQoS
	}
	return nil
}

// InstanceSizeSnapshot represents a snapshot of InstanceSize configuration
// at the time of VM approval. This ensures that modifying an InstanceSize
// does NOT affect existing VMs (ADR-0018 §Immutability).
type InstanceSizeSnapshot struct {
	Name          string                 `json:"name"`
	CPU           int                    `json:"cpu"`
	MemoryMB      int                    `json:"memory_mb"`
	GPUCount      int                    `json:"gpu_count,omitempty"`
	HugepagesGB   int                    `json:"hugepages_gb,omitempty"`
	CPUOvercommit float64                `json:"cpu_overcommit,omitempty"`
	MemOvercommit float64                `json:"mem_overcommit,omitempty"`
	SpecOverrides map[string]interface{} `json:"spec_overrides,omitempty"`
	SnapshotAt    time.Time              `json:"snapshot_at"`
}

// ToSnapshot creates an immutable snapshot of this InstanceSize.
func (i *InstanceSize) ToSnapshot() *InstanceSizeSnapshot {
	return &InstanceSizeSnapshot{
		Name:          i.Name,
		CPU:           i.CPU,
		MemoryMB:      i.MemoryMB,
		GPUCount:      i.GPUCount,
		HugepagesGB:   i.HugepagesGB,
		CPUOvercommit: i.CPUOvercommit,
		MemOvercommit: i.MemOvercommit,
		SpecOverrides: i.SpecOverrides,
		SnapshotAt:    time.Now(),
	}
}

// Errors

var (
	// ErrDedicatedCPURequiresGuaranteedQoS is returned when attempting to
	// use overcommit with dedicated CPU placement.
	// Per KubeVirt documentation, dedicatedCpuPlacement requires Guaranteed QoS.
	ErrDedicatedCPURequiresGuaranteedQoS = errors.New("dedicated CPU requires Guaranteed QoS (overcommit must be 1.0)")
)
