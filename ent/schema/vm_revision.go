package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// VMRevision holds the schema definition for the VMRevision entity.
// Tracks VM configuration version history for audit and rollback.
type VMRevision struct {
	ent.Schema
}

// Mixin of the VMRevision.
func (VMRevision) Mixin() []ent.Mixin {
	return []ent.Mixin{
		AuditMixin{}, // Append-only: created_at only
	}
}

// Fields of the VMRevision.
func (VMRevision) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.Int("revision").
			Positive().
			Immutable(),
		field.JSON("spec", map[string]interface{}{}).
			Optional(), // Full VM spec snapshot at this revision
		field.String("change_reason").
			Optional(),
		field.String("changed_by").
			NotEmpty().
			Immutable(),
	}
}

// Edges of the VMRevision.
func (VMRevision) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("vm", VM.Type).
			Ref("revisions").
			Unique().
			Required(),
	}
}

// Indexes of the VMRevision.
func (VMRevision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("revision").
			Edges("vm").
			Unique(),
	}
}
