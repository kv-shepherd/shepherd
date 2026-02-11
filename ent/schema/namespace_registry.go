package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// NamespaceRegistry holds the schema definition for the NamespaceRegistry entity.
// Namespace is a Shepherd-managed logical entity, NOT bound to any single K8s cluster.
//
// ADR-0017: No cluster_id field. Namespace ↔ Cluster binding occurs at VM approval time.
// ADR-0015 §15: Environment is explicitly set by admin (test/prod).
//
// When a VM is approved, the admin selects the target cluster. If the namespace
// doesn't exist on that cluster, Shepherd creates it JIT (Just-In-Time).
type NamespaceRegistry struct {
	ent.Schema
}

// Mixin of the NamespaceRegistry.
func (NamespaceRegistry) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the NamespaceRegistry.
func (NamespaceRegistry) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty().
			MaxLen(63). // K8s namespace name limit
			Unique().   // Globally unique in Shepherd
			Comment("Globally unique namespace name (RFC 1035, ADR-0019)"),
		field.Enum("environment").
			Values("test", "prod").
			Comment("Explicit environment type, set by admin (ADR-0015 §15)"),
		field.String("description").
			Optional().
			MaxLen(512),
		field.String("created_by").
			NotEmpty(),
		field.Bool("enabled").
			Default(true),
	}
}

// Indexes of the NamespaceRegistry.
func (NamespaceRegistry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("environment"),
		index.Fields("enabled"),
	}
}
