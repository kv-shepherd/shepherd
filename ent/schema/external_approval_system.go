package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExternalApprovalSystem holds the schema definition for the ExternalApprovalSystem entity.
// RFC-0004: V1 scope is interface + schema only. External adapters are V2+ roadmap.
type ExternalApprovalSystem struct {
	ent.Schema
}

// Mixin of the ExternalApprovalSystem.
func (ExternalApprovalSystem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the ExternalApprovalSystem.
func (ExternalApprovalSystem) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.Enum("system_type").
			Values("webhook", "jira", "servicenow", "custom"),
		field.JSON("config", map[string]interface{}{}).
			Sensitive(), // Webhook URL, auth tokens, etc.
		field.Bool("enabled").
			Default(false), // Disabled by default; built-in approval is fallback
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the ExternalApprovalSystem.
func (ExternalApprovalSystem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
	}
}
