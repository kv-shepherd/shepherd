package handlers

import (
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"kv-shepherd.io/shepherd/ent/auditlog"
	"kv-shepherd.io/shepherd/ent/predicate"
)

func auditDetailsDecisionEquals(value string) predicate.AuditLog {
	return auditDetailsValueEquals(value, "decision")
}

func auditDetailsPlacementReasonCodeEquals(value string) predicate.AuditLog {
	return auditDetailsValueEquals(value, "placement_evaluation", "reason_code")
}

func auditDetailsPlacementAdvisoryCodeEquals(value string) predicate.AuditLog {
	return auditDetailsValueEquals(value, "placement_evaluation", "advisory_code")
}

func auditDetailsValueEquals(value string, path ...string) predicate.AuditLog {
	return predicate.AuditLog(func(s *entsql.Selector) {
		s.Where(sqljson.ValueEQ(auditlog.FieldDetails, value, sqljson.Path(path...)))
	})
}
