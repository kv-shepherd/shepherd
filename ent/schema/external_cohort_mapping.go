package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExternalCohortMapping maps normalized external cohorts into platform RBAC targets.
type ExternalCohortMapping struct {
	ent.Schema
}

// Mixin of the ExternalCohortMapping.
func (ExternalCohortMapping) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the ExternalCohortMapping.
func (ExternalCohortMapping) Fields() []ent.Field {
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
		field.String("role_id").
			NotEmpty(),
		field.String("scope_type").
			Optional(),
		field.String("scope_id").
			Optional(),
		field.JSON("allowed_environments", []string{}).
			Optional(),
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the ExternalCohortMapping.
func (ExternalCohortMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "cohort_kind", "cohort_key"),
		index.Fields("provider_id", "cohort_kind", "cohort_key", "role_id", "scope_type", "scope_id").Unique(),
		index.Fields("provider_id", "role_id"),
	}
}
