package handlers

import (
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"kv-shepherd.io/shepherd/ent/predicate"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
)

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
