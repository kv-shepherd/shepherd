package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BatchApprovalTicket holds parent-level batch operation projection.
//
// ADR-0015 §19: parent-child batch model with persisted aggregate counters.
type BatchApprovalTicket struct {
	ent.Schema
}

// Mixin of the BatchApprovalTicket.
func (BatchApprovalTicket) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the BatchApprovalTicket.
func (BatchApprovalTicket) Fields() []ent.Field {
	return []ent.Field{
		// Reuse parent ApprovalTicket ID as stable parent batch ID.
		field.String("id").
			Unique().
			Immutable(),
		field.Enum("batch_type").
			Values("BATCH_CREATE", "BATCH_DELETE", "BATCH_APPROVE", "BATCH_POWER").
			Default("BATCH_CREATE"),
		field.Int("child_count").
			Default(0).
			NonNegative(),
		field.Int("success_count").
			Default(0).
			NonNegative(),
		field.Int("failed_count").
			Default(0).
			NonNegative(),
		field.Int("pending_count").
			Default(0).
			NonNegative(),
		field.Enum("status").
			Values("PENDING_APPROVAL", "IN_PROGRESS", "COMPLETED", "PARTIAL_SUCCESS", "FAILED", "CANCELLED").
			Default("PENDING_APPROVAL"),
		field.String("request_id").
			Optional().
			Nillable(),
		field.String("created_by").
			NotEmpty().
			Immutable(),
		field.String("reason").
			Optional(),
	}
}

// Indexes of the BatchApprovalTicket.
func (BatchApprovalTicket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("created_by"),
		index.Fields("created_at"),
		index.Fields("batch_type", "created_by"),
	}
}
