// Package batchreplay owns the exact normalization and PostgreSQL lookup
// representation for opaque batch idempotency keys.
package batchreplay

import (
	"crypto/sha256"
	"strings"
)

const (
	// TrimCutset is the Unicode White_Space set used by strings.TrimSpace in the
	// Go toolchain pinned by this repository. It is explicit so PostgreSQL and Go
	// normalize historical keys identically across upgrades.
	TrimCutset = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

	// PostgreSQLTrimCutsetLiteral is the SQL E-string form of TrimCutset. Keep
	// this constant byte-for-byte aligned with TrimCutset; integration tests
	// exercise Unicode-trimmed historical replay through the indexed predicate.
	PostgreSQLTrimCutsetLiteral = `E'\u0009\u000a\u000b\u000c\u000d\u0020\u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000'`

	// CandidateLimit bounds historical-duplicate graph loads for one exact
	// normalized key. The SQL lookup applies both the digest predicate and exact
	// BTRIM equality, so hash collisions are excluded before this limit is
	// counted and cannot authorize replay.
	CandidateLimit = 64

	LookupIndexName  = "batch_tickets_replay_lookup_idx"
	HashFunctionName = "shepherd_batch_replay_sha256"

	// EnsureHashFunctionSQL wraps PostgreSQL's stable convert_to function in an
	// immutable UTF-8-specific function. The fixed target encoding makes the
	// result safe for use in an expression index.
	EnsureHashFunctionSQL = `CREATE OR REPLACE FUNCTION "shepherd_batch_replay_sha256"(value text)
RETURNS bytea
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
SET search_path = pg_catalog
RETURN pg_catalog.sha256(pg_catalog.convert_to(value, 'UTF8'))`

	// EnsureLookupIndexSQL is also represented by the governed Atlas migration.
	// SHA-256 is only a lookup accelerator: authorization and idempotency always
	// recheck the exact normalized key and durable payload after the index scan.
	EnsureLookupIndexSQL = `CREATE INDEX IF NOT EXISTS "batch_tickets_replay_lookup_idx"
ON "batch_tickets" (
  "created_by",
  "batch_type",
  "shepherd_batch_replay_sha256"(BTRIM("request_id", ` + PostgreSQLTrimCutsetLiteral + `))
)
WHERE "request_id" IS NOT NULL`
)

// Normalize removes only the shared boundary whitespace and preserves the
// opaque key contents without truncation or rewriting persisted history.
func Normalize(value string) string {
	return strings.Trim(value, TrimCutset)
}

// Digest returns the non-secret SHA-256 lookup key for a normalized request ID.
// The digest is never trusted as identity; callers must exact-check Normalize.
func Digest(value string) []byte {
	sum := sha256.Sum256([]byte(Normalize(value)))
	return sum[:]
}
