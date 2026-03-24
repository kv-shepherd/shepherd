package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserDirectoryProfile stores non-authoritative raw directory attributes.
type UserDirectoryProfile struct {
	ent.Schema
}

func (UserDirectoryProfile) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (UserDirectoryProfile) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("user_id").
			Unique().
			Immutable(),
		field.JSON("attributes", map[string]interface{}{}).
			Comment("Raw provider-specific directory attributes; informational only"),
		field.Time("last_synced_at"),
	}
}

func (UserDirectoryProfile) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("directory_profile").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (UserDirectoryProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Unique(),
	}
}
