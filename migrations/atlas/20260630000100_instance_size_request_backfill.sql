-- Backfill explicit InstanceSize request columns before enforcing request
-- presence in API, approval validation, and VM rendering.

UPDATE "instance_sizes"
SET "cpu_request" = "cpu_cores"
WHERE "cpu_request" IS NULL OR "cpu_request" <= 0;

UPDATE "instance_sizes"
SET "memory_request_gi" = "memory_gi"
WHERE "memory_request_gi" IS NULL
   OR "memory_request_gi" <= 0
   OR (
       "memory_request_gi" IS DISTINCT FROM "memory_gi"
       AND (
           "requires_hugepages" = TRUE
           OR COALESCE(BTRIM("hugepages_size"), '') <> ''
           OR COALESCE("spec_overrides"::text, '') ILIKE '%hugepages%'
       )
   );

ALTER TABLE "instance_sizes"
    ALTER COLUMN "cpu_request" SET NOT NULL,
    ALTER COLUMN "memory_request_gi" SET NOT NULL;
