package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// System holds the schema definition for the System entity.
// ADR-0015 §1: System is a logical business grouping, decoupled from namespace/environment.
// Permissions managed via RoleBinding table, NOT entity fields.
type System struct {
	ent.Schema
}

// Mixin of the System.
func (System) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the System.
func (System) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty().
			MaxLen(15), // ADR-0015 §16: max 15 characters
		field.String("description").
			Optional(),
		field.String("created_by").
			NotEmpty(),
		// Multi-tenancy reserved (ADR-0015)
		field.String("tenant_id").
			Default("default").
			Immutable(),
		// NOTE: No namespace field (ADR-0015 §1)
		// NOTE: No environment field (ADR-0015 §1)
		// NOTE: No maintainers field - use RoleBinding table (ADR-0015 §22)
	}
}

// Edges of the System.
func (System) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("services", Service.Type),
	}
}

// Indexes of the System.
func (System) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(), // Globally unique (ADR-0015 §16)
	}
}
