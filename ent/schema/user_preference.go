package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserPreference stores generic per-user UI/runtime preferences owned by the user.
type UserPreference struct {
	ent.Schema
}

func (UserPreference) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (UserPreference) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("user_id").
			NotEmpty().
			Immutable(),
		field.String("key").
			NotEmpty().
			MaxLen(120),
		field.JSON("value", map[string]interface{}{}),
	}
}

func (UserPreference) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("preferences").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (UserPreference) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "key").Unique(),
		index.Fields("user_id"),
	}
}
