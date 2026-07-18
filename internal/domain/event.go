package domain

import (
	"encoding/json"
	"strings"
	"time"
)

// EventType defines the type of domain event.
type EventType string

const (
	// BatchChildMaxAttempts caps logical child dispatches across the initial
	// attempt and explicit retry requests. River may retry an individual
	// dispatch internally, but that does not create another logical attempt.
	BatchChildMaxAttempts = 3
	// VM Creation Events
	EventVMCreationRequested EventType = "VM_CREATION_REQUESTED"
	EventVMCreationCompleted EventType = "VM_CREATION_COMPLETED"
	EventVMCreationFailed    EventType = "VM_CREATION_FAILED"

	// VM Modification Events
	EventVMModifyRequested EventType = "VM_MODIFY_REQUESTED"
	EventVMModifyCompleted EventType = "VM_MODIFY_COMPLETED"
	EventVMModifyFailed    EventType = "VM_MODIFY_FAILED"

	// VM Deletion Events
	EventVMDeletionRequested EventType = "VM_DELETION_REQUESTED"
	EventVMDeletionCompleted EventType = "VM_DELETION_COMPLETED"
	EventVMDeletionFailed    EventType = "VM_DELETION_FAILED"

	// Power Operations (ADR-0015 §6)
	EventVMStartRequested   EventType = "VM_START_REQUESTED"
	EventVMStartCompleted   EventType = "VM_START_COMPLETED"
	EventVMStartFailed      EventType = "VM_START_FAILED"
	EventVMStopRequested    EventType = "VM_STOP_REQUESTED"
	EventVMStopCompleted    EventType = "VM_STOP_COMPLETED"
	EventVMStopFailed       EventType = "VM_STOP_FAILED"
	EventVMRestartRequested EventType = "VM_RESTART_REQUESTED"
	EventVMRestartCompleted EventType = "VM_RESTART_COMPLETED"
	EventVMRestartFailed    EventType = "VM_RESTART_FAILED"

	// Batch Operations (ADR-0015 §19)
	EventBatchCreateRequested EventType = "BATCH_CREATE_REQUESTED"
	EventBatchCreateCompleted EventType = "BATCH_CREATE_COMPLETED"
	EventBatchCreateFailed    EventType = "BATCH_CREATE_FAILED"
	EventBatchDeleteRequested EventType = "BATCH_DELETE_REQUESTED"
	EventBatchDeleteCompleted EventType = "BATCH_DELETE_COMPLETED"
	EventBatchDeleteFailed    EventType = "BATCH_DELETE_FAILED"
	EventBatchModifyRequested EventType = "BATCH_MODIFY_REQUESTED"
	EventBatchModifyCompleted EventType = "BATCH_MODIFY_COMPLETED"
	EventBatchModifyFailed    EventType = "BATCH_MODIFY_FAILED"
	EventBatchPowerRequested  EventType = "BATCH_POWER_REQUESTED"
	EventBatchPowerCompleted  EventType = "BATCH_POWER_COMPLETED"
	EventBatchPowerFailed     EventType = "BATCH_POWER_FAILED"

	// Request Lifecycle (ADR-0015 §10)
	EventRequestCancelled EventType = "REQUEST_CANCELLED"

	// VNC Access (ADR-0015 §18)
	EventVNCAccessRequested EventType = "VNC_ACCESS_REQUESTED"
	EventVNCAccessGranted   EventType = "VNC_ACCESS_GRANTED"

	// System/Service Events
	EventSystemCreated  EventType = "SYSTEM_CREATED"
	EventSystemDeleted  EventType = "SYSTEM_DELETED"
	EventServiceCreated EventType = "SERVICE_CREATED"
	EventServiceDeleted EventType = "SERVICE_DELETED"
)

// EventStatus defines the status of a domain event.
type EventStatus string

const (
	EventStatusPending    EventStatus = "PENDING"
	EventStatusProcessing EventStatus = "PROCESSING"
	EventStatusCompleted  EventStatus = "COMPLETED"
	EventStatusFailed     EventStatus = "FAILED"
	EventStatusCancelled  EventStatus = "CANCELLED"
)

// DomainEvent represents an immutable domain event (ADR-0009).
type DomainEvent struct {
	EventID       string      `json:"event_id"`
	EventType     EventType   `json:"event_type"`
	AggregateType string      `json:"aggregate_type"`
	AggregateID   string      `json:"aggregate_id"`
	Payload       []byte      `json:"payload"`
	Status        EventStatus `json:"status"`
	CreatedBy     string      `json:"created_by"`
	CreatedAt     time.Time   `json:"created_at"`
	ArchivedAt    *time.Time  `json:"archived_at,omitempty"`
}

// VMCreationPayload is the payload for VM creation events.
// ADR-0015 §3: No SystemID field. ADR-0017: No ClusterID in user request.
type VMCreationPayload struct {
	RequesterID      string  `json:"requester_id"` // User who submitted the request (maps to VM.created_by)
	OwnerID          string  `json:"owner_id,omitempty"`
	ServiceID        string  `json:"service_id"`
	ServiceName      string  `json:"service_name,omitempty"`
	SystemID         string  `json:"system_id,omitempty"`
	SystemName       string  `json:"system_name,omitempty"`
	TemplateID       string  `json:"template_id"`
	TemplateName     string  `json:"template_name,omitempty"`
	InstanceSizeID   string  `json:"instance_size_id"`
	InstanceSizeName string  `json:"instance_size_name,omitempty"`
	Namespace        string  `json:"namespace"`
	Reason           string  `json:"reason"`
	OwnerDisplayName string  `json:"owner_display_name,omitempty"`
	OwnerUsername    string  `json:"owner_username,omitempty"`
	TargetCPUCores   float64 `json:"target_cpu_cores,omitempty"`
	TargetMemoryGi   float64 `json:"target_memory_gi,omitempty"`
	TargetDiskGB     int     `json:"target_disk_gb,omitempty"`
}

// ToJSON converts payload to JSON bytes.
func (p VMCreationPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// ModifiedSpec contains admin modifications (full replacement, not diff).
type ModifiedSpec struct {
	ClusterID      *string `json:"cluster_id,omitempty"`
	InstanceSizeID *string `json:"instance_size_id,omitempty"`
	TemplateID     *string `json:"template_id,omitempty"`
	StorageClass   *string `json:"storage_class,omitempty"`
	ModifiedBy     string  `json:"modified_by"`
	ModifiedReason string  `json:"modified_reason"`
}

// VMDeletePayload is the payload for VM deletion events.
type VMDeletePayload struct {
	VMID               string  `json:"vm_id"`
	VMName             string  `json:"vm_name"`
	ClusterID          string  `json:"cluster_id"`
	ClusterName        string  `json:"cluster_name,omitempty"`
	ClusterEnvironment string  `json:"cluster_environment,omitempty"`
	Namespace          string  `json:"namespace"`
	SystemID           string  `json:"system_id,omitempty"`
	SystemName         string  `json:"system_name,omitempty"`
	ServiceID          string  `json:"service_id,omitempty"`
	ServiceName        string  `json:"service_name,omitempty"`
	OwnerID            string  `json:"owner_id,omitempty"`
	OwnerDisplayName   string  `json:"owner_display_name,omitempty"`
	OwnerUsername      string  `json:"owner_username,omitempty"`
	TemplateID         string  `json:"template_id,omitempty"`
	TemplateName       string  `json:"template_name,omitempty"`
	InstanceSizeID     string  `json:"instance_size_id,omitempty"`
	InstanceSizeName   string  `json:"instance_size_name,omitempty"`
	RequestVMStatus    string  `json:"request_vm_status,omitempty"`
	CurrentCPUCores    float64 `json:"current_cpu_cores,omitempty"`
	CurrentMemoryGi    float64 `json:"current_memory_gi,omitempty"`
	CurrentDiskGB      int     `json:"current_disk_gb,omitempty"`
	Actor              string  `json:"actor"`
}

// ToJSON converts payload to JSON bytes.
func (p VMDeletePayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// VMPowerDispatchMode is immutable submission provenance for a power event.
type VMPowerDispatchMode string

const (
	VMPowerDispatchDirect VMPowerDispatchMode = "direct"
	VMPowerDispatchTicket VMPowerDispatchMode = "ticket"
)

// VMPowerPayload is the payload for VM power operation events.
type VMPowerPayload struct {
	VMID               string  `json:"vm_id"`
	VMName             string  `json:"vm_name"`
	ClusterID          string  `json:"cluster_id"`
	ClusterName        string  `json:"cluster_name,omitempty"`
	ClusterEnvironment string  `json:"cluster_environment,omitempty"`
	Namespace          string  `json:"namespace"`
	SystemID           string  `json:"system_id,omitempty"`
	SystemName         string  `json:"system_name,omitempty"`
	ServiceID          string  `json:"service_id,omitempty"`
	ServiceName        string  `json:"service_name,omitempty"`
	OwnerID            string  `json:"owner_id,omitempty"`
	OwnerDisplayName   string  `json:"owner_display_name,omitempty"`
	OwnerUsername      string  `json:"owner_username,omitempty"`
	TemplateID         string  `json:"template_id,omitempty"`
	TemplateName       string  `json:"template_name,omitempty"`
	InstanceSizeID     string  `json:"instance_size_id,omitempty"`
	InstanceSizeName   string  `json:"instance_size_name,omitempty"`
	RequestVMStatus    string  `json:"request_vm_status,omitempty"`
	CurrentCPUCores    float64 `json:"current_cpu_cores,omitempty"`
	CurrentMemoryGi    float64 `json:"current_memory_gi,omitempty"`
	CurrentDiskGB      int     `json:"current_disk_gb,omitempty"`
	Operation          string  `json:"operation"` // start, stop, restart
	Actor              string  `json:"actor"`
	// DispatchMode makes direct and ticket-backed events distinguishable even
	// if a durable ticket binding is later missing. The field is deliberately
	// not optional: workers fail closed on unknown or legacy-empty values.
	DispatchMode VMPowerDispatchMode `json:"dispatch_mode"`
}

// ToJSON converts payload to JSON bytes.
func (p VMPowerPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// VMModifyPayload is the payload for VM resource change events.
//
// V1 scope:
//   - running VMs use live-update when possible
//   - shrink or non-hotplug topology changes are saved to the VM spec and take
//     effect after the next restart
//   - stopped VMs may accept CPU and memory reconfiguration in either direction
//   - disk remains expansion-only
//   - cpu/memory/disk fields are optional individually, but at least one target
//     value must be provided by the caller
type VMModifyPayload struct {
	VMID               string `json:"vm_id"`
	VMName             string `json:"vm_name"`
	ClusterID          string `json:"cluster_id"`
	ClusterName        string `json:"cluster_name,omitempty"`
	ClusterEnvironment string `json:"cluster_environment,omitempty"`
	Namespace          string `json:"namespace"`
	SystemID           string `json:"system_id,omitempty"`
	SystemName         string `json:"system_name,omitempty"`
	ServiceID          string `json:"service_id,omitempty"`
	ServiceName        string `json:"service_name,omitempty"`
	OwnerID            string `json:"owner_id,omitempty"`
	OwnerDisplayName   string `json:"owner_display_name,omitempty"`
	OwnerUsername      string `json:"owner_username,omitempty"`
	TemplateID         string `json:"template_id,omitempty"`
	TemplateName       string `json:"template_name,omitempty"`
	InstanceSizeID     string `json:"instance_size_id,omitempty"`
	InstanceSizeName   string `json:"instance_size_name,omitempty"`
	RequestVMStatus    string `json:"request_vm_status,omitempty"`
	Actor              string `json:"actor"`

	CurrentCPUCores        float64 `json:"current_cpu_cores"`
	CurrentMemoryGi        float64 `json:"current_memory_gi"`
	CurrentDiskGB          int     `json:"current_disk_gb,omitempty"`
	CurrentCPURequest      float64 `json:"current_cpu_request,omitempty"`
	CurrentMemoryRequestGi float64 `json:"current_memory_request_gi,omitempty"`
	HugepagesPageSize      string  `json:"hugepages_page_size,omitempty"`

	TargetCPUCores  *float64 `json:"target_cpu_cores,omitempty"`
	TargetMemoryGi  *float64 `json:"target_memory_gi,omitempty"`
	TargetDiskGB    *int     `json:"target_disk_gb,omitempty"`
	RequiresRestart bool     `json:"requires_restart,omitempty"`
	ApplyMode       string   `json:"apply_mode,omitempty"`
}

// ToJSON converts payload to JSON bytes.
func (p VMModifyPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// BatchVMItemPayload represents one child item in a batch request.
type BatchVMItemPayload struct {
	VMID               string   `json:"vm_id,omitempty"`
	VMName             string   `json:"vm_name,omitempty"`
	SystemID           string   `json:"system_id,omitempty"`
	SystemName         string   `json:"system_name,omitempty"`
	ServiceID          string   `json:"service_id,omitempty"`
	ServiceName        string   `json:"service_name,omitempty"`
	TemplateID         string   `json:"template_id,omitempty"`
	TemplateName       string   `json:"template_name,omitempty"`
	InstanceSizeID     string   `json:"instance_size_id,omitempty"`
	InstanceSizeName   string   `json:"instance_size_name,omitempty"`
	Namespace          string   `json:"namespace,omitempty"`
	ClusterID          string   `json:"cluster_id,omitempty"`
	ClusterName        string   `json:"cluster_name,omitempty"`
	ClusterEnvironment string   `json:"cluster_environment,omitempty"`
	OwnerID            string   `json:"owner_id,omitempty"`
	OwnerDisplayName   string   `json:"owner_display_name,omitempty"`
	OwnerUsername      string   `json:"owner_username,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	RequestVMStatus    string   `json:"request_vm_status,omitempty"`
	CurrentCPUCores    float64  `json:"current_cpu_cores,omitempty"`
	CurrentMemoryGi    float64  `json:"current_memory_gi,omitempty"`
	CurrentDiskGB      int      `json:"current_disk_gb,omitempty"`
	Operation          string   `json:"operation,omitempty"`
	TargetCPUCores     *float64 `json:"target_cpu_cores,omitempty"`
	TargetMemoryGi     *float64 `json:"target_memory_gi,omitempty"`
	TargetDiskGB       *int     `json:"target_disk_gb,omitempty"`
}

// BatchVMRequestPayload is the parent payload for batch submit requests.
type BatchVMRequestPayload struct {
	Operation   string               `json:"operation"`
	RequestID   string               `json:"request_id,omitempty"`
	Reason      string               `json:"reason,omitempty"`
	SubmittedBy string               `json:"submitted_by"`
	SubmittedAt time.Time            `json:"submitted_at"`
	Items       []BatchVMItemPayload `json:"items"`
}

// BatchApprovalExecutionOptions is the durable approval input used by the
// batch-dispatch claim-check job. The job itself carries only the parent ticket
// ID and reloads these values from the parent ticket snapshot.
type BatchApprovalExecutionOptions struct {
	ClusterID       string   `json:"cluster_id,omitempty"`
	StorageClass    string   `json:"storage_class,omitempty"`
	DVAccessModes   []string `json:"dv_access_modes,omitempty"`
	DVVolumeMode    string   `json:"dv_volume_mode,omitempty"`
	EnableOverride  bool     `json:"enable_override,omitempty"`
	CPURequest      float64  `json:"cpu_request,omitempty"`
	CPULimit        float64  `json:"cpu_limit,omitempty"`
	MemoryRequestGi float64  `json:"memory_request_gi,omitempty"`
	MemoryLimitGi   float64  `json:"memory_limit_gi,omitempty"`
	DiskGB          int      `json:"disk_gb,omitempty"`
}

// BatchApprovalClaimInput describes the initial parent transition and durable
// dispatcher insertion that must commit atomically.
type BatchApprovalClaimInput struct {
	ParentTicketID string
	ParentEventID  string
	Approver       string
	Execution      BatchApprovalExecutionOptions
}

// BatchApprovalDispatchGuard binds one child mutation to the exact durable
// parent/child identity graph validated by the dispatcher. Child writers
// revalidate this fingerprint while holding the parent mutation lock in the
// same transaction that changes state and inserts River work.
type BatchApprovalDispatchGuard struct {
	ParentTicketID        string
	ParentEventID         string
	GraphFingerprint      string
	ChildInputFingerprint map[string]string
	Approver              string
	Execution             BatchApprovalExecutionOptions
}

// BatchApprovalDispatchGraphInvalidError marks a durable identity/state graph
// violation. Transient database errors intentionally use other error types so
// the River worker retries instead of cancelling valid work.
type BatchApprovalDispatchGraphInvalidError struct {
	Detail string
}

func (e *BatchApprovalDispatchGraphInvalidError) Error() string {
	if e == nil || strings.TrimSpace(e.Detail) == "" {
		return "batch approval dispatch graph is invalid"
	}
	return "batch approval dispatch graph is invalid: " + strings.TrimSpace(e.Detail)
}

// Public batch dispatch failure reasons are stable machine-readable values.
// They are safe to persist and return to requesters; untrusted provider and
// infrastructure error text must never cross that boundary.
const (
	BatchApprovalDispatchFailureValidation  = "BATCH_APPROVAL_DISPATCH_VALIDATION_FAILED"
	BatchApprovalDispatchFailureUnsupported = "BATCH_APPROVAL_DISPATCH_UNSUPPORTED_OPERATION"
	BatchApprovalDispatchFailureExhausted   = "BATCH_APPROVAL_DISPATCH_RETRIES_EXHAUSTED"
)

// BatchApprovalRetryChild identifies one failed child to reset as part of a
// durable generic batch retry intent.
type BatchApprovalRetryChild struct {
	TicketID string
	EventID  string
}

// BatchApprovalRetryInput describes the generic retry state changes and
// dispatcher insertion that must commit atomically.
type BatchApprovalRetryInput struct {
	ParentTicketID string
	ParentEventID  string
	Approver       string
	Children       []BatchApprovalRetryChild
	Execution      BatchApprovalExecutionOptions
}

// ToJSON converts payload to JSON bytes.
func (p BatchVMRequestPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}
