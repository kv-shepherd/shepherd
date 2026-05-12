-- Catalog compatibility labels for template/instance-size pairing.
--
-- Empty or NULL labels are interpreted by the application as ["os:any"] for
-- backward compatibility, but existing rows are backfilled to make the default
-- visible to administrators.

ALTER TABLE "templates"
    ADD COLUMN IF NOT EXISTS "system_labels" jsonb DEFAULT '["os:any"]'::jsonb;

ALTER TABLE "instance_sizes"
    ADD COLUMN IF NOT EXISTS "system_labels" jsonb DEFAULT '["os:any"]'::jsonb;

UPDATE "templates"
SET "system_labels" = '["os:any"]'::jsonb
WHERE "system_labels" IS NULL;

UPDATE "instance_sizes"
SET "system_labels" = '["os:any"]'::jsonb
WHERE "system_labels" IS NULL;
