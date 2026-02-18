package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RateLimitExemption stores admin-granted user exemptions for batch submission limits.
//
// ADR-0015 §19: trusted internal users can be exempted from user-level throttles.
type RateLimitExemption struct {
	ent.Schema
}

// Mixin of the RateLimitExemption.
func (RateLimitExemption) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the RateLimitExemption.
func (RateLimitExemption) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("exempted_by").
			NotEmpty(),
		field.String("reason").
			Optional(),
		field.Time("expires_at").
			Optional().
			Nillable(),
	}
}

// Indexes of the RateLimitExemption.
func (RateLimitExemption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("expires_at"),
		index.Fields("created_at"),
	}
}
