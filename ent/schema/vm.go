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
				"STARTING", // Existing VM starting from stopped/paused state
				"RUNNING",  // VM is running
				"STOPPING", // Graceful shutdown in progress
				"STOPPED",  // VM is stopped
				"DELETING", // Being deleted (K8s cleanup)
				"FAILED",   // Error state (was "ERROR", renamed per master-flow)
				// Extended states (K8s/KubeVirt specific)
				"PENDING",   // K8s scheduler waiting
				"MIGRATING", // Live migration in progress
				"PAUSED",    // VM paused
				"UNKNOWN",   // Cluster unreachable or API error
				"NOT_FOUND", // Cluster responded OK but VM resource no longer exists in K8s
			).
			Default("CREATING"), // VM row created at approval → initial status is CREATING
		field.String("hostname").
			Optional(), // Generated: {namespace}-{system}-{service}-{instance}
		field.String("created_by").
			NotEmpty(),
		field.String("ticket_id").
			Optional(), // Reference to the originating ticket
		field.String("root_volume_storage_class").
			Optional().
			Comment("Resolved root-volume storageClass captured at approval time"),
		field.JSON("root_volume_access_modes", []string{}).
			Optional().
			Comment("Resolved root-volume accessModes captured at approval time"),
		field.String("root_volume_volume_mode").
			Optional().
			Comment("Resolved root-volume volumeMode captured at approval time"),
		// NOTE: No system_id field (ADR-0015 §3) — resolve via service.system edge

		// ── ADR-0038: Adaptive K8s VM Status Polling ───────────────────────────────
		// polling_tier drives the interval at which the status-sync River Worker
		// schedules its next execution. "high" = transitional state (≤15s),
		// "low" = stable state (≥30min). New VMs start at "high" to detect
		// provisioning completion quickly.
		field.Enum("polling_tier").
			Values("high", "low").
			Default("high").
			Comment("ADR-0038: polling tier — high for transitional VMs, low for stable VMs"),

		// poll_interval_sec is the concrete interval (seconds) derived from polling_tier.
		// Stored explicitly so the Worker can read it without a switch statement.
		// Default 15 matches the high-frequency tier for newly created VMs.
		field.Int("poll_interval_sec").
			Default(15).
			Comment("ADR-0038: polling interval in seconds (15 for high-tier, 1800 for low-tier)"),

		// last_k8s_rv caches the K8s resourceVersion from the last successful List/Get.
		// All subsequent K8s requests MUST include this value to route through the
		// watch cache and avoid penetrating etcd (ADR-0038 §ResourceVersion requirement).
		// NULL = first poll (no prior baseline); use resourceVersion:"" on next list.
		field.String("last_k8s_rv").
			Optional().
			Nillable().
			Comment("ADR-0038: K8s resourceVersion cache — prevents etcd penetration on routine polls"),

		// last_polled_at records the timestamp of the last successful K8s status sync.
		// Used to detect auto-downgrade: if a VM stays in transitional state for >30min,
		// the Worker downgrades it to low-frequency tier and marks an error hint.
		field.Time("last_polled_at").
			Optional().
			Nillable().
			Comment("ADR-0038: timestamp of last successful K8s status sync"),

		// high_tier_since records when the VM entered high-frequency polling.
		// This mirrors Kubernetes condition.lastTransitionTime semantics and is
		// used for deterministic auto-downgrade checks across all transitional states.
		field.Time("high_tier_since").
			Optional().
			Nillable().
			Comment("ADR-0038: timestamp when VM entered high-frequency polling tier"),
		// ── End ADR-0038 fields ────────────────────────────────────────────────────
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
		// ADR-0038: polling_tier index — River Worker queries VMs by tier
		// to schedule next poll jobs in bulk without full-table scan.
		index.Fields("polling_tier"),
	}
}
