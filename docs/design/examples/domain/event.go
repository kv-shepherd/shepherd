// Package domain provides domain models and event patterns.
//
// ADR-0009: Domain Event Pattern (Claim Check, not Event Sourcing)
// River Job only carries EventID, full payload stored in DomainEvent table.
package domain

import (
	"encoding/json"
	"time"
)

// EventType defines the type of domain event.
type EventType string

const (
	EventVMCreationRequested EventType = "VM_CREATION_REQUESTED"
	EventVMDeletionRequested EventType = "VM_DELETION_REQUESTED"
	EventVMModifyRequested   EventType = "VM_MODIFY_REQUESTED"
	EventVMCreationCompleted EventType = "VM_CREATION_COMPLETED"
	EventVMCreationFailed    EventType = "VM_CREATION_FAILED"
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

// DomainEvent represents an immutable domain event.
//
// Key Constraints (ADR-0009):
// 1. Payload is IMMUTABLE (append-only)
// 2. Modifications stored in ApprovalTicket.ModifiedSpec (full replacement, not diff)
// 3. Worker calls GetEffectiveSpec() to get final config
type DomainEvent struct {
	EventID       string      `json:"event_id"`
	EventType     EventType   `json:"event_type"`
	AggregateType string      `json:"aggregate_type"`
	AggregateID   string      `json:"aggregate_id"`
	Payload       []byte      `json:"payload"` // Immutable JSON
	Status        EventStatus `json:"status"`
	CreatedBy     string      `json:"created_by"`
	CreatedAt     time.Time   `json:"created_at"`
	ArchivedAt    *time.Time  `json:"archived_at"` // Soft archive for cleanup
}

// VMCreationPayload is the payload for VM creation events.
type VMCreationPayload struct {
	SystemID   string `json:"system_id"`
	ServiceID  string `json:"service_id"`
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	CPU        int    `json:"cpu"`
	MemoryMB   int    `json:"memory_mb"`
	DiskGB     int    `json:"disk_gb,omitempty"`
	Reason     string `json:"reason"`
}

// ToJSON converts payload to JSON bytes.
func (p VMCreationPayload) ToJSON() []byte {
	data, _ := json.Marshal(p)
	return data
}

// ModifiedSpec contains admin modifications.
// This is a FULL replacement, not a diff.
type ModifiedSpec struct {
	CPU            *int    `json:"cpu,omitempty"`
	MemoryMB       *int    `json:"memory_mb,omitempty"`
	DiskGB         *int    `json:"disk_gb,omitempty"`
	TemplateID     *string `json:"template_id,omitempty"`
	ModifiedBy     string  `json:"modified_by"`
	ModifiedReason string  `json:"modified_reason"`
}

// ToJSON converts modified spec to JSON bytes.
func (m *ModifiedSpec) ToJSON() []byte {
	if m == nil {
		return nil
	}
	data, _ := json.Marshal(m)
	return data
}

// GetEffectiveSpec returns the final spec to use.
// Uses ModifiedSpec if present, otherwise original payload.
//
// Key Pattern: Full replacement, NOT merge.
// This avoids complex nested structure merging issues.
func GetEffectiveSpec(originalPayload []byte, modifiedSpec []byte) (*VMCreationPayload, error) {
	var original VMCreationPayload
	if err := json.Unmarshal(originalPayload, &original); err != nil {
		return nil, err
	}

	// No modification, use original
	if modifiedSpec == nil {
		return &original, nil
	}

	// Apply modifications (full field replacement)
	var mods ModifiedSpec
	if err := json.Unmarshal(modifiedSpec, &mods); err != nil {
		return nil, err
	}

	result := original
	if mods.CPU != nil {
		result.CPU = *mods.CPU
	}
	if mods.MemoryMB != nil {
		result.MemoryMB = *mods.MemoryMB
	}
	if mods.DiskGB != nil {
		result.DiskGB = *mods.DiskGB
	}
	if mods.TemplateID != nil {
		result.TemplateID = *mods.TemplateID
	}

	return &result, nil
}
