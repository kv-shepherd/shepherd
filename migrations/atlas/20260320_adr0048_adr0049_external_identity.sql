-- ADR-0048 / ADR-0049: external cohort model, directory sync, and JIT profile projection.
--
-- Project status: pre-launch. Remove the old group-only IdP tables instead of
-- keeping compatibility shims.

DROP TABLE IF EXISTS "idp_group_mappings";
DROP TABLE IF EXISTS "idp_synced_groups";

CREATE TABLE IF NOT EXISTS "external_cohorts" (
    "id"            character varying NOT NULL,
    "created_at"    timestamptz       NOT NULL,
    "updated_at"    timestamptz       NOT NULL,
    "provider_id"   character varying NOT NULL,
    "kind"          character varying NOT NULL,
    "key"           character varying NOT NULL,
    "display_name"  character varying DEFAULT NULL,
    "source_field"  character varying DEFAULT NULL,
    "description"   character varying DEFAULT NULL,
    "last_synced_at" timestamptz      DEFAULT NULL,
    PRIMARY KEY ("id")
);

CREATE UNIQUE INDEX IF NOT EXISTS "externalcohort_provider_id_kind_key"
    ON "external_cohorts" ("provider_id", "kind", "key");
CREATE INDEX IF NOT EXISTS "externalcohort_provider_id_kind"
    ON "external_cohorts" ("provider_id", "kind");

CREATE TABLE IF NOT EXISTS "external_cohort_mappings" (
    "id"                   character varying NOT NULL,
    "created_at"           timestamptz       NOT NULL,
    "updated_at"           timestamptz       NOT NULL,
    "provider_id"          character varying NOT NULL,
    "cohort_kind"          character varying NOT NULL,
    "cohort_key"           character varying NOT NULL,
    "cohort_display_name"  character varying DEFAULT NULL,
    "role_id"              character varying NOT NULL,
    "scope_type"           character varying DEFAULT NULL,
    "scope_id"             character varying DEFAULT NULL,
    "allowed_environments" jsonb             DEFAULT NULL,
    "created_by"           character varying NOT NULL,
    PRIMARY KEY ("id")
);

CREATE UNIQUE INDEX IF NOT EXISTS "externalcohortmapping_provider_id_cohort_kind_cohort_key"
    ON "external_cohort_mappings" ("provider_id", "cohort_kind", "cohort_key");
CREATE INDEX IF NOT EXISTS "externalcohortmapping_provider_id_role_id"
    ON "external_cohort_mappings" ("provider_id", "role_id");

CREATE TABLE IF NOT EXISTS "external_cohort_grants" (
    "id"               character varying NOT NULL,
    "created_at"       timestamptz       NOT NULL,
    "updated_at"       timestamptz       NOT NULL,
    "provider_id"      character varying NOT NULL,
    "mapping_id"       character varying NOT NULL,
    "user_id"          character varying NOT NULL,
    "role_binding_id"  character varying NOT NULL,
    "cohort_kind"      character varying NOT NULL,
    "cohort_key"       character varying NOT NULL,
    "managed_by"       character varying NOT NULL DEFAULT 'external_cohort',
    PRIMARY KEY ("id")
);

CREATE UNIQUE INDEX IF NOT EXISTS "externalcohortgrant_mapping_id_user_id"
    ON "external_cohort_grants" ("mapping_id", "user_id");
CREATE UNIQUE INDEX IF NOT EXISTS "externalcohortgrant_role_binding_id"
    ON "external_cohort_grants" ("role_binding_id");
CREATE INDEX IF NOT EXISTS "externalcohortgrant_user_id"
    ON "external_cohort_grants" ("user_id");

CREATE TABLE IF NOT EXISTS "user_directory_profiles" (
    "id"             character varying NOT NULL,
    "created_at"     timestamptz       NOT NULL,
    "updated_at"     timestamptz       NOT NULL,
    "user_id"        character varying NOT NULL,
    "attributes"     jsonb             NOT NULL,
    "last_synced_at" timestamptz       NOT NULL,
    PRIMARY KEY ("id"),
    CONSTRAINT "user_directory_profiles_users_directory_profile"
        FOREIGN KEY ("user_id") REFERENCES "users" ("id")
);

CREATE UNIQUE INDEX IF NOT EXISTS "userdirectoryprofile_user_id"
    ON "user_directory_profiles" ("user_id");

CREATE TABLE IF NOT EXISTS "directory_sync_jobs" (
    "id"                  character varying NOT NULL,
    "created_at"          timestamptz       NOT NULL,
    "updated_at"          timestamptz       NOT NULL,
    "auth_provider_id"    character varying NOT NULL,
    "status"              character varying NOT NULL DEFAULT 'pending',
    "request_snapshot"    jsonb             NOT NULL,
    "conflict_resolution" character varying NOT NULL DEFAULT 'skip',
    "total_entries"       integer           NOT NULL DEFAULT 0,
    "create_count"        integer           NOT NULL DEFAULT 0,
    "update_count"        integer           NOT NULL DEFAULT 0,
    "blocked_count"       integer           NOT NULL DEFAULT 0,
    "error_count"         integer           NOT NULL DEFAULT 0,
    "errors"              jsonb             DEFAULT NULL,
    "triggered_by"        character varying NOT NULL,
    "started_at"          timestamptz       DEFAULT NULL,
    "completed_at"        timestamptz       DEFAULT NULL,
    PRIMARY KEY ("id")
);

CREATE INDEX IF NOT EXISTS "directorysyncjob_auth_provider_id_status"
    ON "directory_sync_jobs" ("auth_provider_id", "status");
CREATE INDEX IF NOT EXISTS "directorysyncjob_auth_provider_id_created_at"
    ON "directory_sync_jobs" ("auth_provider_id", "created_at");
