package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Ticket holds the schema definition for the Ticket entity.
// ADR-0005: Simple approval flow — PENDING → APPROVED or PENDING → REJECTED.
// ADR-0017: Admin-determined placement fields and immutable approval snapshots.
type Ticket struct {
	ent.Schema
}

// Mixin of the Ticket.
func (Ticket) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the Ticket.
func (Ticket) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("event_id").
			NotEmpty().
			Immutable(), // Reference to DomainEvent
		field.Enum("operation_type").
			Values("CREATE", "MODIFY", "DELETE", "POWER", "VNC_ACCESS").
			Default("CREATE"). // Backward compatible; existing tickets are CREATE
			Comment("Distinguishes CREATE, MODIFY, DELETE, POWER, and VNC_ACCESS ticket workflows (Phase 4 governance)"),
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
		field.String("selected_storage_class").
			Optional(),
		field.JSON("template_snapshot", map[string]interface{}{}).
			Optional(), // Full template config at approval time (immutable)
		field.JSON("instance_size_snapshot", map[string]interface{}{}).
			Optional(), // InstanceSize config at approval time (ADR-0018)
		field.JSON("placement_evaluation", map[string]interface{}{}).
			Optional(), // Selected cluster evaluation snapshot (capability + policy verdict)
		field.JSON("modified_spec", map[string]interface{}{}).
			Optional(), // Admin modifications (full replacement, not diff)
		// Batch support
		field.String("parent_ticket_id").
			Optional(), // For batch approval child tickets
		field.Int32("attempt_count").
			Default(0).
			NonNegative().
			Comment("Number of logical dispatch attempts for a batch child ticket (ADR-0015 §19)"),
		field.Time("last_attempt_at").
			Optional().
			Nillable().
			Comment("Time the most recent logical batch-child dispatch attempt began (ADR-0015 §19)"),
	}
}

// Indexes of the Ticket.
func (Ticket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("requester"),
		index.Fields("event_id"),
		index.Fields("parent_ticket_id"),
	}
}

// Annotations enforces persistence invariants that field validators alone
// cannot protect when rows are written through SQL or migrations.
func (Ticket) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Annotation{Checks: map[string]string{
			"tickets_attempt_count_nonnegative": "attempt_count >= 0",
		}},
	}
}
