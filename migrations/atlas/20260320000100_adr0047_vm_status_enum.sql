-- ADR-0047: add STARTING and NOT_FOUND as first-class VM lifecycle states.
--
-- STARTING  -> existing VM booting from STOPPED/PAUSED
-- NOT_FOUND -> cluster responsive, but VM resource absent

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'vm_status') THEN
        CREATE TYPE "vm_status" AS ENUM (
            'CREATING',
            'STARTING',
            'RUNNING',
            'STOPPING',
            'STOPPED',
            'DELETING',
            'FAILED',
            'PENDING',
            'MIGRATING',
            'PAUSED',
            'UNKNOWN',
            'NOT_FOUND'
        );
    END IF;
END $$;

ALTER TYPE "vm_status" ADD VALUE IF NOT EXISTS 'STARTING';
ALTER TYPE "vm_status" ADD VALUE IF NOT EXISTS 'NOT_FOUND';
