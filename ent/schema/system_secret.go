package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SystemSecret holds the schema definition for the SystemSecret entity.
// ADR-0025: Bootstrap secret storage. Only app DB role can access.
type SystemSecret struct {
	ent.Schema
}

// Mixin of the SystemSecret.
func (SystemSecret) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the SystemSecret.
func (SystemSecret) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("key_name").
			NotEmpty(), // e.g. "ENCRYPTION_KEY", "SESSION_SECRET"
		field.String("key_value").
			Sensitive(), // Base64-encoded; encrypted at rest by DB
		field.Enum("source").
			Values("db_generated", "env", "external").
			Default("db_generated"),
	}
}

// Indexes of the SystemSecret.
func (SystemSecret) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("key_name").Unique(),
	}
}
