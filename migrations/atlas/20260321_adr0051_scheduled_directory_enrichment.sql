-- ADR-0051: scheduled directory enrichment metadata on directory_sync_jobs.
--
-- Project status: pre-launch. Introduce explicit execution mode and join key
-- metadata so manual imports and scheduled enrichment remain distinct.

ALTER TABLE "directory_sync_jobs"
    ADD COLUMN IF NOT EXISTS "sync_mode" character varying NOT NULL DEFAULT 'manual_import',
    ADD COLUMN IF NOT EXISTS "join_key_type" character varying NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS "directorysyncjob_auth_provider_id_sync_mode_created_at"
    ON "directory_sync_jobs" ("auth_provider_id", "sync_mode", "created_at");

CREATE INDEX IF NOT EXISTS "directorysyncjob_auth_provider_id_sync_mode_status"
    ON "directory_sync_jobs" ("auth_provider_id", "sync_mode", "status");
