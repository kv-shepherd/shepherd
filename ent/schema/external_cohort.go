package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExternalCohort stores non-authoritative provider cohorts discovered or entered by admins.
type ExternalCohort struct {
	ent.Schema
}

// Mixin of the ExternalCohort.
func (ExternalCohort) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the ExternalCohort.
func (ExternalCohort) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("provider_id").
			NotEmpty(),
		field.String("cohort_kind").
			NotEmpty(),
		field.String("cohort_key").
			NotEmpty(),
		field.String("display_name").
			NotEmpty(),
		field.String("source_field").
			Optional(),
		field.String("description").
			Optional(),
		field.Time("last_synced_at").
			Optional().
			Nillable(),
	}
}

// Indexes of the ExternalCohort.
func (ExternalCohort) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "cohort_kind", "cohort_key").Unique(),
		index.Fields("provider_id", "display_name"),
	}
}
