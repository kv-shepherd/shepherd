// Package schema contains Ent schema definitions for KubeVirt Shepherd.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/ent/schema
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

// TimeMixin adds created_at and updated_at fields to schemas.
// Ent best practice: use mixin for shared timestamp fields.
type TimeMixin struct {
	mixin.Schema
}

// Fields of the TimeMixin.
func (TimeMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// AuditMixin adds created_at (immutable, no updated_at) for append-only tables.
type AuditMixin struct {
	mixin.Schema
}

// Fields of the AuditMixin.
func (AuditMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}
