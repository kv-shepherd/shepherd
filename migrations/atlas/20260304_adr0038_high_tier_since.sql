-- ADR-0038 follow-up: precise transitional-state auto-downgrade clock
--
-- Adds high_tier_since to track when a VM entered high-frequency polling.
-- This avoids using mutable fields like last_polled_at for "stuck" detection.

ALTER TABLE "vms"
    ADD COLUMN IF NOT EXISTS "high_tier_since" timestamptz DEFAULT NULL;

