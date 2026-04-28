-- Platform-wide external auth settings.
--
-- Pre-launch project status: introduce a dedicated platform_settings table
-- instead of overloading provider config with deployment-level callback base URL.

CREATE TABLE IF NOT EXISTS "platform_settings" (
    "id"         character varying NOT NULL,
    "created_at" timestamptz       NOT NULL,
    "updated_at" timestamptz       NOT NULL,
    "key"        character varying NOT NULL,
    "value"      jsonb             NOT NULL,
    "updated_by" character varying NOT NULL,
    PRIMARY KEY ("id")
);

CREATE UNIQUE INDEX IF NOT EXISTS "platformsetting_key"
    ON "platform_settings" ("key");
