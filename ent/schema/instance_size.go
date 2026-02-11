package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// InstanceSize holds the schema definition for the InstanceSize entity.
// ADR-0018: Abstraction layer between user-facing size names and actual resource specs.
type InstanceSize struct {
	ent.Schema
}

// Mixin of the InstanceSize.
func (InstanceSize) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the InstanceSize.
func (InstanceSize) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty(), // e.g. "small", "medium", "large"
		field.String("display_name").
			Optional(), // Human-readable name
		field.String("description").
			Optional(),
		field.Int("cpu_cores").
			Positive(),
		field.Int("memory_mb").
			Positive(),
		field.Int("disk_gb").
			Optional().
			Positive(),
		// Overcommit support
		field.Int("cpu_request").
			Optional(). // Defaults to cpu_cores if not set (no overcommit)
			Positive(),
		field.Int("memory_request_mb").
			Optional(). // Defaults to memory_mb if not set
			Positive(),
		// KubeVirt dedicatedCpuPlacement support (ADR-0018)
		// When true, VM requires Guaranteed QoS: CPU request must equal limit.
		// Overcommit (cpu_request != cpu_cores) is a blocking error with dedicated_cpu.
		field.Bool("dedicated_cpu").
			Default(false),
		// Capability extraction fields for approval-time cluster matching (ADR-0018).
		field.Bool("requires_gpu").
			Default(false),
		field.Bool("requires_sriov").
			Default(false),
		field.Bool("requires_hugepages").
			Default(false),
		field.String("hugepages_size").
			Optional(),
		// Full KubeVirt extension fields (JSON Path -> Value), backend stores without semantic merge.
		field.JSON("spec_overrides", map[string]interface{}{}).
			Optional(),
		field.Int("sort_order").
			Default(0), // Display ordering
		field.Bool("enabled").
			Default(true),
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the InstanceSize.
func (InstanceSize) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
		index.Fields("enabled", "sort_order"),
		index.Fields("requires_gpu"),
		index.Fields("requires_sriov"),
		index.Fields("requires_hugepages", "hugepages_size"),
		index.Fields("dedicated_cpu"),
	}
}
