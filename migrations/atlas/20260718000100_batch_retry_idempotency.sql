-- Persist logical batch-child dispatch attempts. Batch request IDs remain
-- unchanged; submission transactions own serialized exact-key replay.

ALTER TABLE "tickets"
    ADD COLUMN IF NOT EXISTS "attempt_count" integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS "last_attempt_at" timestamptz;

-- Preserve the attempt previously inferred by the API for legacy child rows
-- that have already entered an execution outcome. Historical PENDING/REJECTED
-- rows cannot reliably distinguish never-dispatched work from an interrupted
-- retry, so zero remains their conservative lower bound.
UPDATE "tickets"
SET
    "attempt_count" = 1,
    "last_attempt_at" = COALESCE("last_attempt_at", "updated_at", "created_at")
WHERE "parent_ticket_id" IS NOT NULL
  AND "attempt_count" = 0
  AND "status" IN ('APPROVED', 'EXECUTING', 'SUCCESS', 'FAILED');

-- Ent's NonNegative validator does not constrain direct SQL writes. Preserve
-- retry accounting and fail closed on any future corrupted attempt value.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'tickets_attempt_count_nonnegative'
          AND conrelid = 'tickets'::regclass
          AND contype = 'c'
    ) THEN
        ALTER TABLE "tickets"
            ADD CONSTRAINT "tickets_attempt_count_nonnegative"
            CHECK ("attempt_count" >= 0);
    END IF;
END
$$;

-- batch_tickets.request_id intentionally remains unchanged and unbounded. The
-- non-unique SHA-256 expression index accelerates normalized replay lookup
-- without storing or trusting a digest as identity. Exact request ID, payload,
-- actor, and operation checks remain mandatory after the index scan.
CREATE OR REPLACE FUNCTION "shepherd_batch_replay_sha256"(value text)
RETURNS bytea
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
SET search_path = pg_catalog
RETURN pg_catalog.sha256(pg_catalog.convert_to(value, 'UTF8'));

CREATE INDEX IF NOT EXISTS "batch_tickets_replay_lookup_idx"
ON "batch_tickets" (
    "created_by",
    "batch_type",
    "shepherd_batch_replay_sha256"(BTRIM("request_id", E'\u0009\u000a\u000b\u000c\u000d\u0020\u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000'))
)
WHERE "request_id" IS NOT NULL;
