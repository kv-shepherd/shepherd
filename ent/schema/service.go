package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Service holds the schema definition for the Service entity.
// ADR-0015 §2: Name is immutable after creation. No created_by (inherited from System).
// Permissions inherited from parent System via RoleBinding.
type Service struct {
	ent.Schema
}

// Mixin of the Service.
func (Service) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the Service.
func (Service) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty().
			Immutable(). // Cannot change after creation (ADR-0015 §2)
			MaxLen(15),  // ADR-0015 §16
		field.String("description").
			Optional(),
		field.Int("next_instance_index").
			Default(1).
			Positive(),
		// NOTE: No created_by - inherited from System (ADR-0015 §2)
		// NOTE: No maintainers - inherited from System via RoleBinding
	}
}

// Edges of the Service.
func (Service) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("system", System.Type).
			Ref("services").
			Unique().
			Required(),
		edge.To("vms", VM.Type),
	}
}

// Indexes of the Service.
func (Service) Indexes() []ent.Index {
	return []ent.Index{
		// Unique name within parent System (master-flow Stage 4.B)
		index.Fields("name").
			Edges("system").
			Unique(),
	}
}
