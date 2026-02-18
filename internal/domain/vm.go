// Package domain provides domain models for KubeVirt Shepherd.
//
// All provider methods return domain types, NOT K8s types (Anti-Corruption Layer).
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/domain
package domain

import "time"

// VM represents a virtual machine in the domain layer.
type VM struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Cluster   string    `json:"cluster"`
	Status    VMStatus  `json:"status"`
	Spec      VMSpec    `json:"spec"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// VMSpec represents the desired state of a VM.
type VMSpec struct {
	Name     string            `json:"name,omitempty"`
	CPU      int               `json:"cpu"`
	MemoryMB int               `json:"memory_mb"`
	DiskGB   int               `json:"disk_gb,omitempty"`
	Image    string            `json:"image,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	// SpecOverrides carries advanced KubeVirt spec path/value overrides (ADR-0018 Hybrid Model).
	SpecOverrides map[string]interface{} `json:"spec_overrides,omitempty"`
}

// VMStatus represents the current status of a VM.
// Aligned with master-flow.md Part 4 §VM Status State Diagram.
type VMStatus string

const (
	// Primary lifecycle states (master-flow.md Part 4)
	VMStatusCreating VMStatus = "CREATING" // VM being provisioned (post-approval)
	VMStatusRunning  VMStatus = "RUNNING"  // VM is running
	VMStatusStopping VMStatus = "STOPPING" // VM shutting down (transitional)
	VMStatusStopped  VMStatus = "STOPPED"  // VM is stopped
	VMStatusDeleting VMStatus = "DELETING" // VM being deleted (transitional)
	VMStatusFailed   VMStatus = "FAILED"   // VM in error state (terminal until retry)

	// Extended states (K8s/KubeVirt specific, not in master-flow state diagram)
	VMStatusPending   VMStatus = "PENDING"   // K8s: waiting for resources (scheduler)
	VMStatusMigrating VMStatus = "MIGRATING" // Live migration in progress
	VMStatusPaused    VMStatus = "PAUSED"    // VM paused
	VMStatusUnknown   VMStatus = "UNKNOWN"   // Status cannot be determined
)

// VMList represents a paginated list of VMs.
type VMList struct {
	Items      []*VM  `json:"items"`
	TotalCount int    `json:"total_count"`
	Continue   string `json:"continue,omitempty"`
}

// ValidationResult represents the result of a dry-run validation (ADR-0011).
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Snapshot represents a VM snapshot.
type Snapshot struct {
	Name      string    `json:"name"`
	VMName    string    `json:"vm_name"`
	Namespace string    `json:"namespace"`
	Ready     bool      `json:"ready"`
	CreatedAt time.Time `json:"created_at"`
}

// Clone represents a VM clone operation.
type Clone struct {
	Name      string `json:"name"`
	SourceVM  string `json:"source_vm"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
}

// Migration represents a VM live migration.
type Migration struct {
	Name       string `json:"name"`
	VMName     string `json:"vm_name"`
	Namespace  string `json:"namespace"`
	Phase      string `json:"phase"`
	SourceNode string `json:"source_node,omitempty"`
	TargetNode string `json:"target_node,omitempty"`
}

// InstanceType represents a KubeVirt instance type.
type InstanceType struct {
	Name   string `json:"name"`
	CPU    int    `json:"cpu"`
	Memory string `json:"memory"`
}

// Preference represents a KubeVirt preference.
type Preference struct {
	Name string `json:"name"`
}

// ConsoleConnection represents a console connection to a VM.
type ConsoleConnection struct {
	Type  string `json:"type"` // "vnc" or "serial"
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}
