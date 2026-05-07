-- RFC-0004: external approval system registry.

CREATE TABLE IF NOT EXISTS "external_approval_systems" (
    "id"                       character varying NOT NULL,
    "created_at"               timestamptz       NOT NULL,
    "updated_at"               timestamptz       NOT NULL,
    "name"                     character varying NOT NULL,
    "provider_type"            character varying NOT NULL DEFAULT 'webhook',
    "enabled"                  boolean           NOT NULL DEFAULT true,
    "webhook_url"              character varying NOT NULL,
    "webhook_headers"          jsonb             NOT NULL,
    "timeout_seconds"          integer           NOT NULL DEFAULT 30,
    "retry_count"              integer           NOT NULL DEFAULT 3,
    "retry_backoff_seconds"    integer           NOT NULL DEFAULT 2,
    "signing_key_ciphertext"   character varying DEFAULT NULL,
    "encryption_key_id"        character varying DEFAULT NULL,
    "sort_order"               integer           NOT NULL DEFAULT 0,
    "created_by"               character varying NOT NULL,
    PRIMARY KEY ("id"),
    CONSTRAINT "external_approval_systems_timeout_seconds_positive"
        CHECK ("timeout_seconds" > 0),
    CONSTRAINT "external_approval_systems_retry_count_positive"
        CHECK ("retry_count" > 0),
    CONSTRAINT "external_approval_systems_retry_backoff_seconds_positive"
        CHECK ("retry_backoff_seconds" > 0),
    CONSTRAINT "external_approval_systems_provider_type_valid"
        CHECK ("provider_type" IN ('webhook'))
);

CREATE UNIQUE INDEX IF NOT EXISTS "externalapprovalsystem_name"
    ON "external_approval_systems" ("name");
CREATE INDEX IF NOT EXISTS "externalapprovalsystem_provider_type"
    ON "external_approval_systems" ("provider_type");
CREATE INDEX IF NOT EXISTS "externalapprovalsystem_enabled_sort_order_name"
    ON "external_approval_systems" ("enabled", "sort_order", "name");
