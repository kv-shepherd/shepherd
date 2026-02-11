package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuthProvider holds the schema definition for the AuthProvider entity.
// Unified standard provider config for OIDC, LDAP, SSO, WeCom, Feishu, DingTalk.
type AuthProvider struct {
	ent.Schema
}

// Mixin of the AuthProvider.
func (AuthProvider) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the AuthProvider.
func (AuthProvider) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.Enum("auth_type").
			Values("oidc", "ldap", "sso", "wecom", "feishu", "dingtalk"),
		field.JSON("config", map[string]interface{}{}).
			Sensitive(), // Provider-specific config (encrypted fields inside)
		field.Bool("enabled").
			Default(true),
		field.Int("sort_order").
			Default(0),
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the AuthProvider.
func (AuthProvider) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("auth_type"),
	}
}
