package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RateLimitUserOverride stores per-user custom limits for batch submissions.
//
// ADR-0015 §19: administrators can tune user-level limits when defaults are unsuitable.
type RateLimitUserOverride struct {
	ent.Schema
}

// Mixin of the RateLimitUserOverride.
func (RateLimitUserOverride) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the RateLimitUserOverride.
func (RateLimitUserOverride) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.Int("max_pending_parents").
			Optional().
			Nillable().
			Positive(),
		field.Int("max_pending_children").
			Optional().
			Nillable().
			Positive(),
		field.Int("cooldown_seconds").
			Optional().
			Nillable().
			Min(0),
		field.String("reason").
			Optional(),
		field.String("updated_by").
			NotEmpty(),
	}
}

// Indexes of the RateLimitUserOverride.
func (RateLimitUserOverride) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("updated_by"),
		index.Fields("updated_at"),
	}
}
