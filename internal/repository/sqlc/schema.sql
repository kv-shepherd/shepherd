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

CREATE TABLE approval_tickets (
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
    selected_template_version integer,
    selected_storage_class text,
    template_snapshot jsonb,
    instance_size_snapshot jsonb,
    modified_spec jsonb,
    parent_ticket_id text
);

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
    service_vms text NOT NULL REFERENCES services(id)
);
