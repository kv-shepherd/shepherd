package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditLog holds the schema definition for the AuditLog entity.
// Append-only compliance records. Hard-delete is NOT allowed.
type AuditLog struct {
	ent.Schema
}

// Mixin of the AuditLog.
func (AuditLog) Mixin() []ent.Mixin {
	return []ent.Mixin{
		AuditMixin{}, // Append-only: created_at only
	}
}

// Fields of the AuditLog.
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("action").
			NotEmpty().
			Immutable(), // e.g. "vm.create", "system.delete"
		field.String("resource_type").
			NotEmpty().
			Immutable(), // e.g. "vm", "system", "service"
		field.String("resource_id").
			NotEmpty().
			Immutable(),
		field.String("actor").
			NotEmpty().
			Immutable(), // User who performed the action
		field.JSON("details", map[string]interface{}{}).
			Optional(), // Additional context (before/after state)
		field.String("ip_address").
			Optional().
			Immutable(),
	}
}

// Indexes of the AuditLog.
func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("resource_type", "resource_id"),
		index.Fields("actor"),
		index.Fields("created_at"),
	}
}
