package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Role holds the schema definition for the Role entity.
// ADR-0015 §22, ADR-0019: Role = bundle of permissions. No wildcard permissions.
type Role struct {
	ent.Schema
}

// Mixin of the Role.
func (Role) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the Role.
func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty(), // e.g. "PlatformAdmin", "Operator", "Viewer"
		field.String("display_name").
			Optional(),
		field.String("description").
			Optional(),
		field.JSON("permissions", []string{}).
			Optional(), // e.g. ["vm:read", "vm:create", "system:read"]
		field.Bool("built_in").
			Default(false). // Seed-created roles cannot be deleted
			Immutable(),
		field.Bool("enabled").
			Default(true),
	}
}

// Edges of the Role.
func (Role) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("role_bindings", RoleBinding.Type),
	}
}

// Indexes of the Role.
func (Role) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
	}
}
