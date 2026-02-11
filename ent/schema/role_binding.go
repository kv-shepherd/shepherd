package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RoleBinding holds the schema definition for the RoleBinding entity.
// ADR-0015 §22, ADR-0018 §7: User-role assignments with optional scope.
type RoleBinding struct {
	ent.Schema
}

// Mixin of the RoleBinding.
func (RoleBinding) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the RoleBinding.
func (RoleBinding) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("scope_type").
			Optional(), // e.g. "global", "system", "service"
		field.String("scope_id").
			Optional(), // ID of the scoped resource
		field.JSON("allowed_environments", []string{}).
			Optional(), // Environment-based permission control
		field.String("created_by").
			NotEmpty(),
	}
}

// Edges of the RoleBinding.
func (RoleBinding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("role_bindings").
			Unique().
			Required(),
		edge.From("role", Role.Type).
			Ref("role_bindings").
			Unique().
			Required(),
	}
}

// Indexes of the RoleBinding.
func (RoleBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope_type", "scope_id"),
	}
}
