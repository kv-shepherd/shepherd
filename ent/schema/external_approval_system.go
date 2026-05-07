package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ExternalApprovalSystem holds administrator-managed external approval adapters.
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
		field.Enum("provider_type").
			Values("webhook").
			Default("webhook"),
		field.Bool("enabled").
			Default(true),
		field.String("webhook_url").
			NotEmpty(),
		field.JSON("webhook_headers", map[string]string{}).
			Sensitive(),
		field.Int("timeout_seconds").
			Positive().
			Default(30),
		field.Int("retry_count").
			Positive().
			Default(3),
		field.Int("retry_backoff_seconds").
			Positive().
			Default(2),
		field.String("signing_key_ciphertext").
			Optional().
			Sensitive().
			Comment("AES-256-GCM protected external approval webhook signing key"),
		field.String("encryption_key_id").
			Optional().
			Comment("Identifier of the key used to protect signing_key_ciphertext"),
		field.Int("sort_order").
			Default(0),
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the ExternalApprovalSystem.
func (ExternalApprovalSystem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("provider_type"),
		index.Fields("enabled", "sort_order", "name"),
	}
}
