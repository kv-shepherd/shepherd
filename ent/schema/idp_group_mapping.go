package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// IdPGroupMapping holds the schema definition for the IdPGroupMapping entity.
// master-flow Stage 2.C: Maps IdP groups to platform roles for automatic RBAC.
type IdPGroupMapping struct {
	ent.Schema
}

// Mixin of the IdPGroupMapping.
func (IdPGroupMapping) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the IdPGroupMapping.
func (IdPGroupMapping) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("provider_id").
			NotEmpty(), // Reference to AuthProvider
		field.String("external_group_id").
			NotEmpty(), // External group identifier from IdP
		field.String("role_id").
			NotEmpty(), // Reference to Role
		field.String("scope_type").
			Optional(), // Optional scope type (global/system/service/vm)
		field.String("scope_id").
			Optional(), // Optional scope id
		field.JSON("allowed_environments", []string{}).
			Optional(), // Environment constraints (test/prod)
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the IdPGroupMapping.
func (IdPGroupMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "external_group_id").Unique(),
		index.Fields("provider_id", "role_id"),
	}
}
