package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ApprovalPolicy holds the schema definition for the ApprovalPolicy entity.
// ADR-0005: Defines environment-level policies. V1 scope: PENDING → APPROVED/REJECTED only.
type ApprovalPolicy struct {
	ent.Schema
}

// Mixin of the ApprovalPolicy.
func (ApprovalPolicy) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
	}
}

// Fields of the ApprovalPolicy.
func (ApprovalPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.String("name").
			NotEmpty(),
		field.String("description").
			Optional(),
		field.Enum("action").
			Values("VM_CREATE", "VM_DELETE", "VM_MODIFY").
			Default("VM_CREATE"),
		field.String("namespace_pattern").
			Optional(), // Regex pattern to match namespaces requiring approval
		field.Bool("enabled").
			Default(true),
		field.String("created_by").
			NotEmpty(),
	}
}

// Indexes of the ApprovalPolicy.
func (ApprovalPolicy) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("action", "enabled"),
	}
}
