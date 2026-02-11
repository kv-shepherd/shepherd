package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User holds the schema definition for the User entity.
// ADR-0018: Platform user accounts (local + IdP-linked).
type User struct {
	ent.Schema
}

// Mixin of the User.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("username").
			NotEmpty().
			MaxLen(255),
		field.String("email").
			Optional().
			MaxLen(255),
		field.String("display_name").
			Optional(),
		field.String("password_hash").
			Optional().
			Sensitive(), // For local auth
		field.Bool("force_password_change").
			Default(false), // ADR-0018: admin/admin requires forced change
		field.String("auth_provider_id").
			Optional(), // Link to auth_providers for SSO/OIDC
		field.String("external_id").
			Optional(), // Stable external subject identifier
		field.Bool("enabled").
			Default(true),
		field.Time("last_login_at").
			Optional().
			Nillable(),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("role_bindings", RoleBinding.Type),
		edge.To("notifications", Notification.Type),
	}
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("username").Unique(),
		index.Fields("email").Unique(),
		index.Fields("auth_provider_id", "external_id").Unique(),
	}
}
