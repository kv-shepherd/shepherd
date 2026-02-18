package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// IdPSyncedGroup holds the schema definition for the IdPSyncedGroup entity.
// master-flow Stage 2.C: Groups synced from external identity providers.
type IdPSyncedGroup struct {
	ent.Schema
}

// Mixin of the IdPSyncedGroup.
func (IdPSyncedGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the IdPSyncedGroup.
func (IdPSyncedGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("provider_id").
			NotEmpty(), // Reference to AuthProvider
		field.String("external_group_id").
			NotEmpty(), // Group ID from IdP
		field.String("group_name").
			NotEmpty(),
		field.String("source_field").
			Optional(), // Claim/attribute field used for syncing (e.g. groups, department)
		field.String("description").
			Optional(),
		field.Time("last_synced_at").
			Optional().
			Nillable(),
	}
}

// Indexes of the IdPSyncedGroup.
func (IdPSyncedGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "external_group_id").Unique(),
	}
}
