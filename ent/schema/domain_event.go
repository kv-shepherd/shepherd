package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DomainEvent holds the schema definition for the DomainEvent entity.
// ADR-0009: Claim-check pattern. Payload is IMMUTABLE (append-only).
// River job carries only EventID; full payload stored here.
type DomainEvent struct {
	ent.Schema
}

// Mixin of the DomainEvent.
func (DomainEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		AuditMixin{}, // Append-only: created_at only
	}
}

// Fields of the DomainEvent.
func (DomainEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("event_type").
			NotEmpty().
			Immutable(), // e.g. "VM_CREATION_REQUESTED"
		field.String("aggregate_type").
			NotEmpty().
			Immutable(), // e.g. "vm", "system"
		field.String("aggregate_id").
			NotEmpty().
			Immutable(),
		field.Bytes("payload").
			Immutable(), // Immutable JSON (ADR-0009)
		field.Enum("status").
			Values("PENDING", "PROCESSING", "COMPLETED", "FAILED", "CANCELLED").
			Default("PENDING"),
		field.String("created_by").
			NotEmpty().
			Immutable(),
		field.Time("archived_at").
			Optional().
			Nillable(), // Soft archive for cleanup
	}
}

// Indexes of the DomainEvent.
func (DomainEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("aggregate_type", "aggregate_id"),
		index.Fields("event_type"),
		index.Fields("status"),
		index.Fields("created_at"),
	}
}
