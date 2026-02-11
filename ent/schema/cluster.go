package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Cluster holds the schema definition for the Cluster entity.
// Multi-cluster credential management with encrypted kubeconfig storage.
type Cluster struct {
	ent.Schema
}

// Mixin of the Cluster.
func (Cluster) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the Cluster.
func (Cluster) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty().
			MaxLen(63),
		field.String("display_name").
			Optional(),
		field.String("api_server_url").
			NotEmpty(),
		field.Bytes("encrypted_kubeconfig").
			Sensitive(), // AES-256-GCM encrypted
		field.String("encryption_key_id").
			Optional(), // For key rotation support
		field.Enum("status").
			Values("UNKNOWN", "HEALTHY", "UNHEALTHY", "UNREACHABLE").
			Default("UNKNOWN"),
		field.String("kubevirt_version").
			Optional(), // Detected KubeVirt version
		field.JSON("enabled_features", []string{}).
			Optional(), // Detected feature gates
		field.String("created_by").
			NotEmpty(),
		field.Enum("environment").
			Values("test", "prod").
			Default("test").
			Comment("Cluster environment type (ADR-0015 §1, §15)"),
		field.JSON("storage_classes", []string{}).
			Optional().
			Comment("Auto-detected StorageClass list from cluster (ADR-0015 §8)"),
		field.String("default_storage_class").
			Optional().
			Comment("Admin-specified default StorageClass"),
		field.Time("storage_classes_updated_at").
			Optional().
			Nillable().
			Comment("Last StorageClass detection timestamp"),
		field.Bool("enabled").
			Default(true),
	}
}

// Indexes of the Cluster.
func (Cluster) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("status"),
	}
}
