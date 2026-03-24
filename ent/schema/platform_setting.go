package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformSetting stores platform-wide runtime settings owned by Shepherd core.
type PlatformSetting struct {
	ent.Schema
}

// Mixin of the PlatformSetting.
func (PlatformSetting) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the PlatformSetting.
func (PlatformSetting) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("key").
			NotEmpty().
			Unique().
			Immutable(),
		field.JSON("value", map[string]interface{}{}),
		field.String("updated_by").
			NotEmpty(),
	}
}

// Indexes of the PlatformSetting.
func (PlatformSetting) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("key").Unique(),
	}
}
