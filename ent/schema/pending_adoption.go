package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PendingAdoption holds the schema definition for the PendingAdoption entity.
// Recovery and compensation: tracks K8s resources that need adoption after failures.
type PendingAdoption struct {
	ent.Schema
}

// Mixin of the PendingAdoption.
func (PendingAdoption) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the PendingAdoption.
func (PendingAdoption) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("cluster_id").
			NotEmpty(),
		field.String("namespace").
			NotEmpty(),
		field.String("resource_name").
			NotEmpty(),
		field.String("resource_type").
			NotEmpty(), // e.g. "VirtualMachine"
		field.Enum("status").
			Values("PENDING", "ADOPTED", "REJECTED", "EXPIRED").
			Default("PENDING"),
		field.String("discovered_by").
			Optional(), // Worker that discovered the orphan
		field.JSON("labels", map[string]string{}).
			Optional(), // K8s labels snapshot
	}
}

// Indexes of the PendingAdoption.
func (PendingAdoption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("cluster_id", "namespace", "resource_name").Unique(),
		index.Fields("status"),
	}
}
