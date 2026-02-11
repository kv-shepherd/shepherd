package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Notification holds the schema definition for the Notification entity.
// V1 implementation: Platform Inbox (database-backed in-app notifications).
//
// ADR-0015 §20: Notifications are synchronous writes within the same
// DB transaction as business operations (NOT via River Queue).
// V2+: External push channels (email, webhook) via River Queue.
type Notification struct {
	ent.Schema
}

// Mixin of the Notification.
func (Notification) Mixin() []ent.Mixin {
	return []ent.Mixin{
		AuditMixin{}, // created_at only (notifications are append-only)
	}
}

// Fields of the Notification.
func (Notification) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			Immutable(),
		field.Enum("type").
			Values(
				"APPROVAL_PENDING",
				"APPROVAL_COMPLETED",
				"APPROVAL_REJECTED",
				"VM_STATUS_CHANGE",
			).
			Comment("Notification type (ADR-0015 §20 trigger points)"),
		field.String("title").
			NotEmpty().
			MaxLen(255),
		field.String("message").
			NotEmpty().
			MaxLen(2048),
		field.String("resource_type").
			Optional().
			Comment("Related resource type (e.g. vm, approval_ticket)"),
		field.String("resource_id").
			Optional().
			Comment("Related resource ID for navigation"),
		field.Bool("read").
			Default(false).
			Comment("Whether the notification has been read"),
		field.Time("read_at").
			Optional().
			Nillable().
			Comment("When the notification was marked as read"),
	}
}

// Edges of the Notification.
func (Notification) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("notifications").
			Unique().
			Required(),
	}
}

// Indexes of the Notification.
func (Notification) Indexes() []ent.Index {
	return []ent.Index{
		index.Edges("user").Fields("read"),       // Fast unread count query
		index.Edges("user").Fields("created_at"), // Paginated list by user
		index.Fields("created_at"),               // Retention cleanup (90 days)
	}
}
