package handlers

import (
	"strings"

	"entgo.io/ent/dialect/sql"

	"kv-shepherd.io/shepherd/ent/predicate"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/ent/userdirectoryprofile"
)

func impossibleUserSearchPredicate() predicate.User {
	return predicate.User(func(s *sql.Selector) {
		s.Where(sql.P(func(builder *sql.Builder) {
			builder.WriteString("1 = 0")
		}))
	})
}

func userProfileAttributeContains(field, value string) predicate.User {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	return entuser.HasDirectoryProfileWith(predicate.UserDirectoryProfile(func(s *sql.Selector) {
		s.Where(sql.P(func(builder *sql.Builder) {
			builder.WriteString("LOWER(")
			writeUserProfileAttributeTextPath(builder, field)
			builder.WriteString(") LIKE ")
			builder.Arg("%" + strings.ToLower(value) + "%")
		}))
	}))
}

func userProfileAttributeEqualFold(field, value string) predicate.User {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	return entuser.HasDirectoryProfileWith(predicate.UserDirectoryProfile(func(s *sql.Selector) {
		s.Where(sql.P(func(builder *sql.Builder) {
			builder.WriteString("LOWER(")
			writeUserProfileAttributeTextPath(builder, field)
			builder.WriteString(") = ")
			builder.Arg(strings.ToLower(value))
		}))
	}))
}

func writeUserProfileAttributeTextPath(builder *sql.Builder, field string) {
	builder.Ident(userdirectoryprofile.FieldAttributes)
	builder.WriteString(" ->> ")
	builder.Arg(field)
}
