package domain

import (
	"encoding/json"
	"time"
)

// EventType defines the type of domain event.
type EventType string

const (
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
	RequesterID    string `json:"requester_id"` // User who submitted the request (maps to VM.created_by)
	ServiceID      string `json:"service_id"`
	TemplateID     string `json:"template_id"`
	InstanceSizeID string `json:"instance_size_id"`
	Namespace      string `json:"namespace"`
	Reason         string `json:"reason"`
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
	VMID      string `json:"vm_id"`
	VMName    string `json:"vm_name"`
	ClusterID string `json:"cluster_id"`
	Namespace string `json:"namespace"`
	Actor     string `json:"actor"`
}

// ToJSON converts payload to JSON bytes.
func (p VMDeletePayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// VMPowerPayload is the payload for VM power operation events.
type VMPowerPayload struct {
	VMID      string `json:"vm_id"`
	VMName    string `json:"vm_name"`
	ClusterID string `json:"cluster_id"`
	Namespace string `json:"namespace"`
	Operation string `json:"operation"` // start, stop, restart
	Actor     string `json:"actor"`
}

// ToJSON converts payload to JSON bytes.
func (p VMPowerPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// BatchVMItemPayload represents one child item in a batch request.
type BatchVMItemPayload struct {
	VMID           string `json:"vm_id,omitempty"`
	ServiceID      string `json:"service_id,omitempty"`
	TemplateID     string `json:"template_id,omitempty"`
	InstanceSizeID string `json:"instance_size_id,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	Reason         string `json:"reason,omitempty"`
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

// ToJSON converts payload to JSON bytes.
func (p BatchVMRequestPayload) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}
