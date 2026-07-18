-- Minimal schema for sqlc query validation (ADR-0012).
-- This file mirrors the core columns used by atomic approval transactions.

CREATE TABLE systems (
    id text PRIMARY KEY,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    name text NOT NULL,
    description text,
    created_by text NOT NULL,
    tenant_id text NOT NULL
);

CREATE TABLE services (
    id text PRIMARY KEY,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    name text NOT NULL,
    description text,
    next_instance_index integer NOT NULL,
    system_services text NOT NULL REFERENCES systems(id)
);

CREATE TABLE domain_events (
    id text PRIMARY KEY,
    created_at timestamptz NOT NULL,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    payload bytea NOT NULL,
    status text NOT NULL,
    created_by text NOT NULL,
    archived_at timestamptz
);

CREATE TABLE tickets (
    id text PRIMARY KEY,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    event_id text NOT NULL,
    operation_type text NOT NULL,
    status text NOT NULL,
    requester text NOT NULL,
    approver text,
    reason text,
    reject_reason text,
    selected_cluster_id text,
    selected_storage_class text,
    template_snapshot jsonb,
    instance_size_snapshot jsonb,
    placement_evaluation jsonb,
    modified_spec jsonb,
    parent_ticket_id text,
    attempt_count integer NOT NULL DEFAULT 0
        CONSTRAINT tickets_attempt_count_nonnegative CHECK (attempt_count >= 0),
    last_attempt_at timestamptz
);

CREATE TABLE batch_tickets (
    id text PRIMARY KEY,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    batch_type text NOT NULL,
    child_count integer NOT NULL,
    success_count integer NOT NULL DEFAULT 0,
    failed_count integer NOT NULL DEFAULT 0,
    pending_count integer NOT NULL,
    status text NOT NULL,
    request_id text,
    created_by text NOT NULL,
    reason text
);

CREATE OR REPLACE FUNCTION "shepherd_batch_replay_sha256"(value text)
RETURNS bytea
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
SET search_path = pg_catalog
RETURN pg_catalog.sha256(pg_catalog.convert_to(value, 'UTF8'));

CREATE INDEX "batch_tickets_replay_lookup_idx"
ON batch_tickets (
    created_by,
    batch_type,
    "shepherd_batch_replay_sha256"(BTRIM(request_id, E'\u0009\u000a\u000b\u000c\u000d\u0020\u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000'))
)
WHERE request_id IS NOT NULL;

CREATE TABLE vms (
    id text PRIMARY KEY,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    name text NOT NULL,
    instance text NOT NULL,
    namespace text NOT NULL,
    cluster_id text,
    status text NOT NULL,
    hostname text,
    created_by text NOT NULL,
    ticket_id text,
    root_volume_storage_class text,
    root_volume_access_modes jsonb,
    root_volume_volume_mode text,
    service_vms text NOT NULL REFERENCES services(id)
);
