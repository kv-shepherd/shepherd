-- ADR-0038: Adaptive K8s VM Status Polling — Phase 1 Schema Migration
--
-- Adds four polling-state fields to the `vms` table to enable state-machine-driven
-- adaptive polling intervals for K8s VM status synchronization.
--
-- Fields added (per ADR-0038 §Storage):
--   polling_tier       ENUM('high','low')  — drives River Worker scheduling frequency
--   poll_interval_sec  INTEGER             — concrete interval derived from polling_tier
--   last_k8s_rv        TEXT (nullable)     — K8s resourceVersion cache; NULL = first poll
--   last_polled_at     TIMESTAMPTZ (null.) — timestamp of last successful K8s status sync
--
-- Index added:
--   vms_polling_tier   — River Worker queries VMs by tier for batch scheduling
--
-- Generated equivalent: atlas migrate diff --env shepherd
-- Author: AI Agent (2026-03-03)
-- Review: ADR-0038 status "proposed", 48hr review until 2026-03-04

-- Step 1: Create the polling_tier ENUM type (PostgreSQL native ENUM).
-- "high" → transitional VMs (Creating/Deleting/Updating) — polling interval ≤ 15s.
-- "low"  → stable VMs (Running/Stopped/Failed)           — polling interval ≥ 30min.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'vm_polling_tier') THEN
        CREATE TYPE "vm_polling_tier" AS ENUM ('high', 'low');
    END IF;
END $$;

-- Step 2: Add polling fields to the vms table.
-- All new columns have safe defaults — no backfill needed for existing rows:
--   polling_tier      DEFAULT 'high'  → existing VMs start as high-frequency (safe) until
--                                       the Worker observes their actual state and transitions.
--   poll_interval_sec DEFAULT 15      → matches the high-tier interval (15 seconds).
--   last_k8s_rv       NULL            → signals "first poll, no prior resourceVersion".
--   last_polled_at    NULL            → signals "never synced".
ALTER TABLE "vms"
    ADD COLUMN IF NOT EXISTS "polling_tier"       "vm_polling_tier" NOT NULL DEFAULT 'high',
    ADD COLUMN IF NOT EXISTS "poll_interval_sec"  integer           NOT NULL DEFAULT 15,
    ADD COLUMN IF NOT EXISTS "last_k8s_rv"        character varying          DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS "last_polled_at"     timestamptz                DEFAULT NULL;

-- Step 3: Add index on polling_tier for efficient tier-based queries.
-- River Worker uses: SELECT id FROM vms WHERE polling_tier = 'high' AND status IN (...)
-- Without this index, a full table scan would occur on large deployments.
CREATE INDEX IF NOT EXISTS "vm_polling_tier"
    ON "vms" ("polling_tier");

-- Migration metadata comment (Atlas convention)
-- atlas:checksum sha256:placeholder-replaced-by-atlas-on-apply
