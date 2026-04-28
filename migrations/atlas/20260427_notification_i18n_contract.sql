-- Add machine-readable i18n message contracts for notification inbox rows.
--
-- Existing English title/message columns remain as fallback/log text. The new
-- key/params columns are the UI contract.

ALTER TABLE "notifications"
    ADD COLUMN IF NOT EXISTS "title_key" character varying NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS "title_params" jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS "message_key" character varying NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS "message_params" jsonb NOT NULL DEFAULT '{}';

UPDATE "notifications"
SET
    "title_key" = 'notification.message.legacy.title',
    "title_params" = jsonb_build_object('text', "title"),
    "message_key" = 'notification.message.legacy.body',
    "message_params" = jsonb_build_object('text', "message")
WHERE "title_key" = '' OR "message_key" = '';

ALTER TABLE "notifications"
    ALTER COLUMN "title_params" DROP DEFAULT,
    ALTER COLUMN "message_params" DROP DEFAULT;
