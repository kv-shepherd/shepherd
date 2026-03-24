package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DirectorySyncJob tracks async directory import jobs per auth provider.
type DirectorySyncJob struct {
	ent.Schema
}

func (DirectorySyncJob) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

func (DirectorySyncJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("auth_provider_id").
			NotEmpty(),
		field.Enum("status").
			Values("pending", "running", "completed", "failed").
			Default("pending"),
		field.JSON("request_snapshot", map[string]interface{}{}).
			Comment("Opaque provider_request payload frozen at trigger time"),
		field.String("conflict_resolution").
			Default("skip"),
		field.Enum("sync_mode").
			Values("manual_import", "scheduled_enrichment").
			Default("manual_import"),
		field.String("join_key_type").
			Default(""),
		field.Int("total_entries").
			Default(0),
		field.Int("create_count").
			Default(0),
		field.Int("update_count").
			Default(0),
		field.Int("blocked_count").
			Default(0),
		field.Int("error_count").
			Default(0),
		field.JSON("errors", []string{}).
			Optional(),
		field.String("triggered_by").
			NotEmpty(),
		field.Time("started_at").
			Optional().
			Nillable(),
		field.Time("completed_at").
			Optional().
			Nillable(),
	}
}

func (DirectorySyncJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("auth_provider_id", "created_at"),
		index.Fields("auth_provider_id", "status"),
		index.Fields("auth_provider_id", "sync_mode", "created_at"),
		index.Fields("auth_provider_id", "sync_mode", "status"),
	}
}
