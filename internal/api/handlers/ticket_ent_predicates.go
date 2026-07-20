package handlers

import (
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/predicate"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/repository/batchreplay"
)

// batchTicketNormalizedRequestIDEquals preserves historical request IDs while
// allowing replay lookups to apply the same boundary trim as new submissions.
// PostgreSQL text remains unbounded and is never rewritten or truncated.
func batchTicketNormalizedRequestIDEquals(value string) predicate.BatchTicket {
	return func(selector *entsql.Selector) {
		selector.Where(entsql.P(func(builder *entsql.Builder) {
			builder.WriteString("\"shepherd_batch_replay_sha256\"(BTRIM(").
				WriteString(selector.C(batchticket.FieldRequestID)).
				WriteString(", ").
				WriteString(batchreplay.PostgreSQLTrimCutsetLiteral).
				WriteString(")) = ").
				Arg(batchreplay.Digest(value))
		}))
		selector.Where(entsql.P(func(builder *entsql.Builder) {
			builder.WriteString("BTRIM(").
				WriteString(selector.C(batchticket.FieldRequestID)).
				WriteString(", ").
				WriteString(batchreplay.PostgreSQLTrimCutsetLiteral).
				WriteString(") = ").
				Arg(value)
		}))
	}
}

func ticketPlacementSelectedClusterNameContains(value string) predicate.Ticket {
	return predicate.Ticket(func(s *entsql.Selector) {
		s.Where(sqljson.StringContains(
			entticket.FieldPlacementEvaluation,
			value,
			sqljson.Path("selected_cluster_name"),
		))
	})
}

func ticketPlacementAdvisoryCodeEquals(value string) predicate.Ticket {
	return predicate.Ticket(func(s *entsql.Selector) {
		s.Where(sqljson.ValueEQ(
			entticket.FieldPlacementEvaluation,
			value,
			sqljson.Path("advisory_code"),
		))
	})
}

// domainEventBoundToBatchChild keeps a domain-event state transition tied to
// the exact child ticket and parent batch proven by the preceding ticket CAS.
// The child row is already locked by that CAS, so this subquery also prevents a
// corrupted or concurrently re-parented ticket from authorizing an unrelated
// event update.
func domainEventBoundToBatchChild(ticketID, parentTicketID string) predicate.DomainEvent {
	return predicate.DomainEvent(func(s *entsql.Selector) {
		tickets := entsql.Table(entticket.Table)
		ticketEventIDs := entsql.Select(tickets.C(entticket.FieldEventID)).
			From(tickets).
			Where(entsql.And(
				entsql.EQ(tickets.C(entticket.FieldID), ticketID),
				entsql.EQ(tickets.C(entticket.FieldParentTicketID), parentTicketID),
			))
		s.Where(entsql.In(s.C(domainevent.FieldID), ticketEventIDs))
	})
}
