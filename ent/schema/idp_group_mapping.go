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
		field.String("synced_group_id").
			NotEmpty(), // Reference to IdPSyncedGroup
		field.String("role_id").
			NotEmpty(), // Reference to Role
		field.String("scope").
			Optional(), // Optional scope constraint (e.g., system_id)
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the IdPGroupMapping.
func (IdPGroupMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("synced_group_id", "role_id").Unique(),
	}
}
