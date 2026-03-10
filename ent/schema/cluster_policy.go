package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ClusterPolicy holds the schema definition for the ClusterPolicy entity.
// ADR-0042: Administrative policy layer separate from detected cluster capability.
type ClusterPolicy struct {
	ent.Schema
}

// Mixin of the ClusterPolicy.
func (ClusterPolicy) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the ClusterPolicy.
func (ClusterPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("cluster_id").
			Unique(),
		field.Bool("allow_cpu_overcommit").
			Default(true),
		field.Bool("allow_memory_overcommit").
			Default(true),
		field.Bool("allow_dedicated_cpu").
			Default(true),
		field.Bool("allow_gpu").
			Default(true),
		field.Bool("allow_sriov").
			Default(true),
		field.Bool("allow_hugepages").
			Default(true),
		field.JSON("allowed_hugepages_sizes", []string{}).
			Optional(),
		field.Bool("allow_cdi_clone").
			Default(true),
		field.JSON("allowed_clone_source_namespaces", []string{}).
			Optional(),
		field.JSON("allowed_storage_classes", []string{}).
			Optional(),
		field.String("created_by").
			NotEmpty(),
		field.String("updated_by").
			Optional(),
	}
}

// Edges of the ClusterPolicy.
func (ClusterPolicy) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("cluster", Cluster.Type).
			Ref("policy").
			Field("cluster_id").
			Unique().
			Required(),
	}
}

// Indexes of the ClusterPolicy.
func (ClusterPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("cluster_id").Unique(),
	}
}
