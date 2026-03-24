package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExternalCohortGrant tracks auto-managed role bindings derived from external cohorts.
type ExternalCohortGrant struct {
	ent.Schema
}

// Mixin of the ExternalCohortGrant.
func (ExternalCohortGrant) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the ExternalCohortGrant.
func (ExternalCohortGrant) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("user_id").
			NotEmpty().
			Immutable(),
		field.String("provider_id").
			NotEmpty().
			Immutable(),
		field.String("binding_key").
			NotEmpty(),
		field.String("role_binding_id").
			NotEmpty(),
		field.JSON("source_mapping_ids", []string{}).
			Optional(),
		field.Time("last_applied_at"),
	}
}

// Edges of the ExternalCohortGrant.
func (ExternalCohortGrant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("external_cohort_grants").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
		edge.From("role_binding", RoleBinding.Type).
			Ref("external_cohort_grants").
			Field("role_binding_id").
			Unique().
			Required(),
	}
}

// Indexes of the ExternalCohortGrant.
func (ExternalCohortGrant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "provider_id", "binding_key").Unique(),
		index.Fields("role_binding_id").Unique(),
	}
}
