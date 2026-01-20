// Package domain provides domain models.
//
// Anti-Corruption Layer: These types are decoupled from K8s types.
// Provider mapper translates between K8s types and domain types.
package domain

import "time"

// VMStatus represents the status of a VM.
type VMStatus string

const (
	VMStatusPending   VMStatus = "PENDING"
	VMStatusRunning   VMStatus = "RUNNING"
	VMStatusStopped   VMStatus = "STOPPED"
	VMStatusFailed    VMStatus = "FAILED"
	VMStatusMigrating VMStatus = "MIGRATING"
	VMStatusPaused    VMStatus = "PAUSED"
	VMStatusUnknown   VMStatus = "UNKNOWN"
)

// VM represents a virtual machine in the domain model.
// Decoupled from kubevirtv1.VirtualMachine.
type VM struct {
	// Identity
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`

	// Governance Model
	SystemID  string `json:"system_id"`
	ServiceID string `json:"service_id"`
	Instance  string `json:"instance"` // e.g., "06"

	// Spec
	CPU      int    `json:"cpu"`
	MemoryMB int    `json:"memory_mb"`
	DiskGB   int    `json:"disk_gb,omitempty"`
	Template string `json:"template,omitempty"`

	// Status
	Status        VMStatus `json:"status"`
	StatusMessage string   `json:"status_message,omitempty"`
	IP            string   `json:"ip,omitempty"`
	NodeName      string   `json:"node_name,omitempty"`

	// Timestamps
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`

	// Metadata
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// VMSpec is the specification for creating/updating a VM.
type VMSpec struct {
	Name      string            `json:"name"`
	CPU       int               `json:"cpu"`
	MemoryMB  int               `json:"memory_mb"`
	DiskGB    int               `json:"disk_gb,omitempty"`
	Template  string            `json:"template"`
	SystemID  string            `json:"system_id"`
	ServiceID string            `json:"service_id"`
	Labels    map[string]string `json:"labels,omitempty"`
	CloudInit *CloudInit        `json:"cloud_init,omitempty"`
}

// CloudInit contains cloud-init configuration.
type CloudInit struct {
	UserData    string `json:"user_data,omitempty"`
	NetworkData string `json:"network_data,omitempty"`
}

// VMList is a list of VMs with pagination info.
type VMList struct {
	Items    []*VM  `json:"items"`
	Total    int    `json:"total"`
	Continue string `json:"continue,omitempty"`
}

// Snapshot represents a VM snapshot.
type Snapshot struct {
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Cluster      string    `json:"cluster"`
	SourceVM     string    `json:"source_vm"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	ReadyToUse   bool      `json:"ready_to_use"`
	SizeBytes    int64     `json:"size_bytes,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// Clone represents a VM clone operation.
type Clone struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Cluster   string    `json:"cluster"`
	SourceVM  string    `json:"source_vm"`
	TargetVM  string    `json:"target_vm"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Migration represents a live migration operation.
type Migration struct {
	Name         string     `json:"name"`
	Namespace    string     `json:"namespace"`
	Cluster      string     `json:"cluster"`
	VMName       string     `json:"vm_name"`
	Status       string     `json:"status"`
	SourceNode   string     `json:"source_node"`
	TargetNode   string     `json:"target_node,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

// InstanceType represents a VM instance type.
type InstanceType struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"` // Empty for cluster-scoped
	CPU         int               `json:"cpu"`
	MemoryMB    int               `json:"memory_mb"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Preference represents a VM preference.
type Preference struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ConsoleConnection contains console connection info.
type ConsoleConnection struct {
	Type     string `json:"type"` // "vnc" or "serial"
	Endpoint string `json:"endpoint"`
	Token    string `json:"token,omitempty"`
}

// ValidationResult contains the result of spec validation.
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}
