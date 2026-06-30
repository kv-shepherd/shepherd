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
		field.Float("cpu_cores").
			Positive(),
		field.Float("memory_gi").
			Positive(),
		field.Int("disk_gb").
			Optional().
			Positive(),
		// Overcommit support
		field.Float("cpu_request").
			Positive(),
		field.Float("memory_request_gi").
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
		// spec_overrides: full KubeVirt extension fields (JSON Path -> Value).
		// Admin-only: omitted from user-facing API responses to avoid leaking
		// internal infrastructure tuning details. See instanceSizeToPublicAPI().
		field.JSON("spec_overrides", map[string]interface{}{}).
			Optional(),
		// dv_access_modes sets the DataVolume PVC access mode(s) for VMs of this size.
		// e.g. ["ReadWriteMany"]. When set, uses CDI 'pvc' format instead of 'storage'.
		// Required for storage backends like Ceph RBD that require specific access modes.
		field.JSON("dv_access_modes", []string{}).
			Optional().
			Comment("DataVolume PVC accessModes, e.g. [\"ReadWriteMany\"]. Empty = CDI default."),
		// dv_volume_mode sets the DataVolume PVC volume mode: "Block" or "Filesystem".
		// Ceph RBD typically requires "Block" mode for optimal performance.
		field.String("dv_volume_mode").
			Optional().
			Default("").
			Comment("DataVolume PVC volumeMode: Block or Filesystem. Empty = CDI default."),
		field.JSON("system_labels", []string{}).
			Optional().
			Comment("Platform-defined compatibility labels. Empty/NULL means os:any."),
		field.Enum("catalog_scope").
			Values("unclassified", "test", "prod", "all").
			Default("unclassified").
			Comment("Catalog visibility scope only. Not scheduling environment."),
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
		index.Fields("catalog_scope"),
	}
}
