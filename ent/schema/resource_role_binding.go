package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ResourceRoleBinding holds the schema definition for the ResourceRoleBinding entity.
// ADR-0018, master-flow Stage 4.A+: Resource-level member management (owner/admin/member/viewer).
type ResourceRoleBinding struct {
	ent.Schema
}

// Mixin of the ResourceRoleBinding.
func (ResourceRoleBinding) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the ResourceRoleBinding.
func (ResourceRoleBinding) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("user_id").
			NotEmpty(),
		field.String("resource_type").
			NotEmpty(), // e.g. "system", "service"
		field.String("resource_id").
			NotEmpty(),
		field.Enum("role").
			Values("owner", "admin", "member", "viewer").
			Default("viewer"),
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the ResourceRoleBinding.
func (ResourceRoleBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "resource_type", "resource_id").Unique(),
		index.Fields("resource_type", "resource_id"),
	}
}
