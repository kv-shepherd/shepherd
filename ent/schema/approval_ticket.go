package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ApprovalTicket holds the schema definition for the ApprovalTicket entity.
// ADR-0005: Simple approval flow — PENDING → APPROVED or PENDING → REJECTED.
// ADR-0017: Admin-determined fields (cluster, template_version, storage_class).
type ApprovalTicket struct {
	ent.Schema
}

// Mixin of the ApprovalTicket.
func (ApprovalTicket) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the ApprovalTicket.
func (ApprovalTicket) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("event_id").
			NotEmpty().
			Immutable(), // Reference to DomainEvent
		field.Enum("operation_type").
			Values("CREATE", "DELETE", "VNC_ACCESS").
			Default("CREATE"). // Backward compatible; existing tickets are CREATE
			Comment("Distinguishes CREATE vs DELETE approval tickets (Phase 4 governance)"),
		field.Enum("status").
			Values("PENDING", "APPROVED", "REJECTED", "CANCELLED", "EXECUTING", "SUCCESS", "FAILED").
			Default("PENDING"),
		field.String("requester").
			NotEmpty().
			Immutable(),
		field.String("approver").
			Optional(), // Set when approved/rejected
		field.String("reason").
			Optional(), // Requester's reason
		field.String("reject_reason").
			Optional(), // Approver's rejection reason
		// Admin-determined fields (ADR-0017)
		field.String("selected_cluster_id").
			Optional(),
		field.Int("selected_template_version").
			Optional(),
		field.String("selected_storage_class").
			Optional(),
		field.JSON("template_snapshot", map[string]interface{}{}).
			Optional(), // Full template config at approval time (immutable)
		field.JSON("instance_size_snapshot", map[string]interface{}{}).
			Optional(), // InstanceSize config at approval time (ADR-0018)
		field.JSON("modified_spec", map[string]interface{}{}).
			Optional(), // Admin modifications (full replacement, not diff)
		// Batch support
		field.String("parent_ticket_id").
			Optional(), // For batch approval child tickets
	}
}

// Indexes of the ApprovalTicket.
func (ApprovalTicket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("requester"),
		index.Fields("event_id"),
		index.Fields("parent_ticket_id"),
	}
}
