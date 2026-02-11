package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Template holds the schema definition for the Template entity.
// ADR-0018: Templates stored in PostgreSQL, not as YAML files.
type Template struct {
	ent.Schema
}

// Mixin of the Template.
func (Template) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the Template.
func (Template) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.String("display_name").
			Optional(),
		field.String("description").
			Optional(),
		field.Int("version").
			Default(1).
			Positive(),
		field.JSON("spec", map[string]interface{}{}).
			Optional(), // Full VM template specification
		field.String("os_family").
			Optional(), // e.g. "linux", "windows"
		field.String("os_version").
			Optional(), // e.g. "ubuntu-22.04"
		field.Bool("enabled").
			Default(true),
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the Template.
func (Template) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "version").Unique(),
		index.Fields("enabled"),
	}
}
