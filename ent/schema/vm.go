package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// VM holds the schema definition for the VM entity.
// ADR-0015 §3: Associates service_id only. No system_id field — obtain via service edge.
type VM struct {
	ent.Schema
}

// Mixin of the VM.
func (VM) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the VM.
func (VM) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty().
			Immutable(), // Platform-generated (ADR-0015 §4)
		field.String("instance").
			NotEmpty(), // Instance number e.g. "01"
		field.String("namespace").
			NotEmpty(), // User-provided, immutable after submission (ADR-0017)
		field.String("cluster_id").
			Optional(), // Set by admin during approval
		field.Enum("status").
			Values(
				// Primary lifecycle states (master-flow.md Part 4)
				"CREATING", // Post-approval, being provisioned
				"RUNNING",  // VM is running
				"STOPPING", // Graceful shutdown in progress
				"STOPPED",  // VM is stopped
				"DELETING", // Being deleted (K8s cleanup)
				"FAILED",   // Error state (was "ERROR", renamed per master-flow)
				// Extended states (K8s/KubeVirt specific)
				"PENDING",   // K8s scheduler waiting
				"MIGRATING", // Live migration in progress
				"PAUSED",    // VM paused
				"UNKNOWN",   // Status undetermined
			).
			Default("CREATING"), // VM row created at approval → initial status is CREATING
		field.String("hostname").
			Optional(), // Generated: {namespace}-{system}-{service}-{instance}
		field.String("created_by").
			NotEmpty(),
		field.String("ticket_id").
			Optional(), // Reference to approval ticket
		// NOTE: No system_id field (ADR-0015 §3) — resolve via service.system edge
	}
}

// Edges of the VM.
func (VM) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("service", Service.Type).
			Ref("vms").
			Unique().
			Required(),
		edge.To("revisions", VMRevision.Type),
	}
}

// Indexes of the VM.
func (VM) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("namespace", "name").Unique(),
		index.Fields("status"),
		index.Fields("cluster_id"),
	}
}
