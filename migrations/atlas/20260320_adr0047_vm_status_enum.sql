-- ADR-0047: add STARTING and NOT_FOUND as first-class VM lifecycle states.
--
-- STARTING  -> existing VM booting from STOPPED/PAUSED
-- NOT_FOUND -> cluster responsive, but VM resource absent

ALTER TYPE "vm_status" ADD VALUE IF NOT EXISTS 'STARTING';
ALTER TYPE "vm_status" ADD VALUE IF NOT EXISTS 'NOT_FOUND';
