// Package domain provides example domain entities for KubeVirt Shepherd.
//
// This file defines the InstanceSize entity per ADR-0018.
// InstanceSize abstracts KubeVirt VM resource configuration using the Hybrid Model.
//
// Reference: docs/adr/ADR-0018-instance-size-abstraction.md

package domain

import (
	"errors"
	"time"
)

// InstanceSize represents a predefined VM resource configuration (ADR-0018 Hybrid Model).
// Admin-managed entity that defines CPU, Memory, and optional overcommit settings.
//
// Key Design Decisions (ADR-0018):
// - Name is globally unique (index.Fields("name").Unique())
// - Core scheduling fields are stored in indexed columns for query performance
// - spec_overrides stores remaining KubeVirt-specific configuration as JSONB
// - Immutability: VMs snapshot InstanceSize at approval time (changes don't affect existing VMs)
type InstanceSize struct {
	ID          string `json:"id"`
	Name        string `json:"name"` // Globally unique, e.g., "small", "medium-gpu"
	Description string `json:"description,omitempty"`

	// ============================================================
	// INDEXED COLUMNS: Core scheduling fields (ADR-0018 Hybrid Model)
	// These are extracted from full config and stored for efficient queries
	// ============================================================

	// CPU Configuration
	CPUCores int `json:"cpu_cores"` // Logical CPUs (indexed)

	// Memory Configuration
	Memory string `json:"memory"` // e.g., "16Gi" (indexed)

	// Hardware Capability Flags (indexed for cluster matching)
	RequiresGPU       bool   `json:"requires_gpu"`             // True if GPU passthrough required
	RequiresSRIOV     bool   `json:"requires_sriov"`           // True if SR-IOV required
	RequiresHugepages bool   `json:"requires_hugepages"`       // True if Hugepages required
	HugepagesSize     string `json:"hugepages_size,omitempty"` // e.g., "2Mi", "1Gi"
	DedicatedCPU      bool   `json:"dedicated_cpu"`            // True if dedicatedCpuPlacement required

	// Overcommit Configuration (ADR-0018 §481-486)
	// Uses request/limit model, NOT ratio model
	CPUOvercommit *OvercommitConfig `json:"cpu_overcommit,omitempty"`
	MemOvercommit *OvercommitConfig `json:"mem_overcommit,omitempty"`

	// ============================================================
	// JSONB EXTENSION: Flexible storage for remaining KubeVirt fields
	// Backend does NOT interpret these contents beyond basic JSON validation
	// ============================================================
	SpecOverrides map[string]interface{} `json:"spec_overrides,omitempty"`

	// Enabled flag for soft-delete
	Enabled bool `json:"enabled"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OvercommitConfig defines request/limit for resource overcommit (ADR-0018 §481-486).
// This struct uses explicit request/limit values, NOT ratio.
//
// QoS Implications:
// - If Enabled = false or Request == Limit → Guaranteed QoS
// - If Enabled = true and Request < Limit → Burstable QoS
type OvercommitConfig struct {
	// Enabled indicates whether overcommit is enabled for this resource
	Enabled bool `json:"enabled"`

	// Request is the Kubernetes resource request value
	// For CPU: "4" (cores), for Memory: "16Gi"
	Request string `json:"request"`

	// Limit is the Kubernetes resource limit value
	// For CPU: "8" (cores), for Memory: "32Gi"
	Limit string `json:"limit"`
}

// IsGuaranteedQoS returns true if this config results in Guaranteed QoS.
// Guaranteed QoS requires: request == limit (or overcommit disabled)
func (c *OvercommitConfig) IsGuaranteedQoS() bool {
	if c == nil || !c.Enabled {
		return true // No overcommit means Guaranteed QoS
	}
	return c.Request == c.Limit
}

// ValidateWithDedicatedCPU checks if overcommit is compatible with dedicated CPU.
// Per KubeVirt documentation, dedicatedCpuPlacement requires Guaranteed QoS.
//
// Returns error if:
// - Dedicated CPU is enabled AND overcommit results in Burstable QoS
func ValidateWithDedicatedCPU(dedicatedCPU bool, cpuOvercommit, memOvercommit *OvercommitConfig) error {
	if !dedicatedCPU {
		return nil // No dedicated CPU, any QoS allowed
	}

	// Check CPU overcommit
	if cpuOvercommit != nil && !cpuOvercommit.IsGuaranteedQoS() {
		return ErrDedicatedCPURequiresGuaranteedQoS
	}

	// Check Memory overcommit (also required for full Guaranteed QoS)
	if memOvercommit != nil && !memOvercommit.IsGuaranteedQoS() {
		return ErrDedicatedCPURequiresGuaranteedQoS
	}

	return nil
}

// InstanceSizeSnapshot represents a snapshot of InstanceSize configuration
// at the time of VM approval. This ensures that modifying an InstanceSize
// does NOT affect existing VMs (ADR-0018 §Immutability).
type InstanceSizeSnapshot struct {
	Name        string `json:"name"`
	CPUCores    int    `json:"cpu_cores"`
	Memory      string `json:"memory"`
	RequiresGPU bool   `json:"requires_gpu,omitempty"`

	// Final computed request/limit values (after overcommit applied)
	FinalCPURequest string `json:"final_cpu_request"`
	FinalCPULimit   string `json:"final_cpu_limit"`
	FinalMemRequest string `json:"final_mem_request"`
	FinalMemLimit   string `json:"final_mem_limit"`

	// Snapshot of spec_overrides for audit
	SpecOverrides map[string]interface{} `json:"spec_overrides,omitempty"`

	SnapshotAt time.Time `json:"snapshot_at"`
}

// ToSnapshot creates an immutable snapshot of this InstanceSize.
// The final request/limit values are computed based on overcommit settings.
func (i *InstanceSize) ToSnapshot() *InstanceSizeSnapshot {
	snapshot := &InstanceSizeSnapshot{
		Name:          i.Name,
		CPUCores:      i.CPUCores,
		Memory:        i.Memory,
		RequiresGPU:   i.RequiresGPU,
		SpecOverrides: i.SpecOverrides,
		SnapshotAt:    time.Now(),
	}

	// Compute final CPU request/limit
	if i.CPUOvercommit != nil && i.CPUOvercommit.Enabled {
		snapshot.FinalCPURequest = i.CPUOvercommit.Request
		snapshot.FinalCPULimit = i.CPUOvercommit.Limit
	} else {
		// No overcommit: request == limit (Guaranteed QoS)
		cpuStr := string(rune(i.CPUCores + '0')) // Simple int to string for example
		snapshot.FinalCPURequest = cpuStr
		snapshot.FinalCPULimit = cpuStr
	}

	// Compute final Memory request/limit
	if i.MemOvercommit != nil && i.MemOvercommit.Enabled {
		snapshot.FinalMemRequest = i.MemOvercommit.Request
		snapshot.FinalMemLimit = i.MemOvercommit.Limit
	} else {
		// No overcommit: request == limit (Guaranteed QoS)
		snapshot.FinalMemRequest = i.Memory
		snapshot.FinalMemLimit = i.Memory
	}

	return snapshot
}

// Errors

var (
	// ErrDedicatedCPURequiresGuaranteedQoS is returned when attempting to
	// use overcommit with dedicated CPU placement.
	// Per KubeVirt documentation, dedicatedCpuPlacement requires Guaranteed QoS.
	ErrDedicatedCPURequiresGuaranteedQoS = errors.New("dedicated CPU requires Guaranteed QoS (request must equal limit)")
)
