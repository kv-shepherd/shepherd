// Package domain provides domain models and event patterns.
//
// ADR-0009: Domain Event Pattern (Claim Check, not Event Sourcing)
// River Job only carries EventID, full payload stored in DomainEvent table.
//
// ADR-0015: Extended event types for governance operations.
// Includes power operations, VNC access, batch operations, notifications.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/domain
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

	// VNC Console Events (ADR-0015 §18)
	EventVNCAccessRequested EventType = "VNC_ACCESS_REQUESTED"
	EventVNCAccessGranted   EventType = "VNC_ACCESS_GRANTED"
	EventVNCAccessDenied    EventType = "VNC_ACCESS_DENIED"
	EventVNCTokenRevoked    EventType = "VNC_TOKEN_REVOKED"

	// Batch Operations (ADR-0015 §19)
	EventBatchCreateRequested EventType = "BATCH_CREATE_REQUESTED"
	EventBatchCreateCompleted EventType = "BATCH_CREATE_COMPLETED"
	EventBatchCreateFailed    EventType = "BATCH_CREATE_FAILED"
	EventBatchDeleteRequested EventType = "BATCH_DELETE_REQUESTED"
	EventBatchDeleteCompleted EventType = "BATCH_DELETE_COMPLETED"
	EventBatchDeleteFailed    EventType = "BATCH_DELETE_FAILED"

	// Request Lifecycle Events (ADR-0015 §10)
	EventRequestCancelled EventType = "REQUEST_CANCELLED"

	// Notification Events (ADR-0015 §20)
	EventNotificationSent EventType = "NOTIFICATION_SENT"

	// System/Service Events (recorded, no approval required)
	EventSystemCreated  EventType = "SYSTEM_CREATED"
	EventSystemDeleted  EventType = "SYSTEM_DELETED"
	EventServiceCreated EventType = "SERVICE_CREATED"
	EventServiceDeleted EventType = "SERVICE_DELETED"
)

// EventStatus defines the status of a domain event.
// Aligned with ADR-0009 DomainEvent Schema (L156).
type EventStatus string

const (
	EventStatusPending    EventStatus = "PENDING"
	EventStatusProcessing EventStatus = "PROCESSING"
	EventStatusCompleted  EventStatus = "COMPLETED" // Per ADR-0009 L156, NOT "SUCCESS"
	EventStatusFailed     EventStatus = "FAILED"
	EventStatusCancelled  EventStatus = "CANCELLED"
)

// DomainEvent represents an immutable domain event.
//
// Key Constraints (ADR-0009, ADR-0012):
// 1. Payload is IMMUTABLE (append-only)
// 2. Modifications stored in ApprovalTicket.ModifiedSpec (full replacement, not diff)
// 3. Worker calls GetEffectiveSpec() to get final config
//
// Database-Level Immutability Enforcement:
//
// Option A: PostgreSQL Trigger (Recommended for defense-in-depth)
//
//	CREATE OR REPLACE FUNCTION prevent_domain_event_payload_update()
//	RETURNS TRIGGER AS $$
//	BEGIN
//	    IF OLD.payload IS DISTINCT FROM NEW.payload THEN
//	        RAISE EXCEPTION 'DomainEvent.payload is immutable - updates are forbidden (ADR-0009)';
//	    END IF;
//	    RETURN NEW;
//	END;
//	$$ LANGUAGE plpgsql;
//
//	CREATE TRIGGER domain_event_payload_immutable
//	    BEFORE UPDATE ON domain_events
//	    FOR EACH ROW
//	    EXECUTE FUNCTION prevent_domain_event_payload_update();
//
// Option B: Ent ORM Hook (Application-level enforcement)
//
//	func (DomainEvent) Hooks() []ent.Hook {
//	    return []ent.Hook{
//	        hook.On(func(next ent.Mutator) ent.Mutator {
//	            return hook.DomainEventFunc(func(ctx context.Context, m *ent.DomainEventMutation) (ent.Value, error) {
//	                if m.Op().Is(ent.OpUpdate | ent.OpUpdateOne) {
//	                    if _, ok := m.Payload(); ok {
//	                        return nil, errors.New("DomainEvent.payload is immutable (ADR-0009)")
//	                    }
//	                }
//	                return next.Mutate(ctx, m)
//	            })
//	        }, ent.OpUpdate|ent.OpUpdateOne),
//	    }
//	}
//
// Note: Both options should be implemented for defense-in-depth.
// The DB trigger catches any bypass attempts (e.g., direct SQL).
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
//
// NOTE (ADR-0015 §3): No SystemID field.
// System is always resolved via ServiceID → Service.Edges.System.
// This ensures Single Source of Truth and prevents data inconsistency.
//
// NOTE (master-flow.md §Stage 3.C): No ClusterID in user request.
// Cluster is selected by admin during approval and stored in ApprovalTicket.ModifiedSpec.
// This prevents users from bypassing capacity planning.
type VMCreationPayload struct {
	ServiceID  string `json:"service_id"`
	TemplateID string `json:"template_id"`
	// NOTE: ClusterID is NOT in user request - selected during approval (master-flow.md)
	// NOTE (ADR-0017): Namespace is user-provided and immutable after submission
	Namespace string `json:"namespace"`
	CPU       int    `json:"cpu"`
	MemoryMB  int    `json:"memory_mb"`
	DiskGB    int    `json:"disk_gb,omitempty"`
	Reason    string `json:"reason"`
	// NOTE: Name is platform-generated, not stored in payload (ADR-0015 §4)
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
// ADR-0005 Decision: Full Replacement Strategy
// When admin modifies a request, they submit a COMPLETE ModifiedSpec.
// This is NOT a merge/patch operation - ModifiedSpec fully replaces
// the relevant fields for audit clarity and state predictability.
//
// Rationale (from best practices research):
// - Audit trail is clearer: each approval shows complete final state
// - No merge conflict or ambiguity about which fields were actually changed
// - Simpler implementation: no complex diff/merge logic needed
//
// ❌ REMOVED: Field-level merge was considered but rejected.
// Merge would require tracking which fields were intentionally null vs unchanged.
func GetEffectiveSpec(originalPayload []byte, modifiedSpec []byte) (*VMCreationPayload, error) {
	var original VMCreationPayload
	if err := json.Unmarshal(originalPayload, &original); err != nil {
		return nil, err
	}

	// No modification, use original
	if modifiedSpec == nil {
		return &original, nil
	}

	// ADR-0005: Full replacement - ModifiedSpec contains complete admin-approved config
	// Admin must provide ALL fields they want in final spec
	var mods ModifiedSpec
	if err := json.Unmarshal(modifiedSpec, &mods); err != nil {
		return nil, err
	}

	// Build result from ModifiedSpec as authoritative source
	// Original values are used ONLY as fallback when ModifiedSpec field is nil
	// This preserves backward compatibility while enforcing full-spec semantics
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
