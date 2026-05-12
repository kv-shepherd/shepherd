-- name: ApproveCreateTicket :execrows
UPDATE tickets
SET
    status = 'APPROVED',
    approver = sqlc.arg(approver),
    selected_cluster_id = sqlc.arg(selected_cluster_id),
    selected_storage_class = CASE
        WHEN sqlc.arg(selected_storage_class)::text = '' THEN selected_storage_class
        ELSE sqlc.arg(selected_storage_class)::text
    END,
    template_snapshot = COALESCE(sqlc.narg(template_snapshot)::jsonb, template_snapshot),
    instance_size_snapshot = COALESCE(sqlc.narg(instance_size_snapshot)::jsonb, instance_size_snapshot),
    placement_evaluation = COALESCE(sqlc.narg(placement_evaluation)::jsonb, placement_evaluation),
    modified_spec = COALESCE(sqlc.narg(modified_spec)::jsonb, modified_spec),
    updated_at = NOW()
WHERE
    id = sqlc.arg(id)
    AND event_id = sqlc.arg(event_id)
    AND status = 'PENDING'
    AND operation_type = 'CREATE';

-- name: ApproveDeleteTicket :execrows
UPDATE tickets
SET
    status = 'APPROVED',
    approver = sqlc.arg(approver),
    updated_at = NOW()
WHERE
    id = sqlc.arg(id)
    AND event_id = sqlc.arg(event_id)
    AND status = 'PENDING'
    AND operation_type = 'DELETE';

-- name: ApproveModifyTicket :execrows
UPDATE tickets
SET
    status = 'APPROVED',
    approver = sqlc.arg(approver),
    modified_spec = COALESCE(sqlc.narg(modified_spec)::jsonb, modified_spec),
    updated_at = NOW()
WHERE
    id = sqlc.arg(id)
    AND event_id = sqlc.arg(event_id)
    AND status = 'PENDING'
    AND operation_type = 'MODIFY';

-- name: ApprovePowerTicket :execrows
UPDATE tickets
SET
    status = 'APPROVED',
    approver = sqlc.arg(approver),
    updated_at = NOW()
WHERE
    id = sqlc.arg(id)
    AND event_id = sqlc.arg(event_id)
    AND status = 'PENDING'
    AND operation_type = 'POWER';

-- name: SetDomainEventStatus :execrows
UPDATE domain_events
SET status = $2
WHERE id = $1;

-- name: ResetPowerRetryTicket :execrows
UPDATE tickets
SET
    status = 'EXECUTING',
    reject_reason = NULL,
    updated_at = NOW()
WHERE
    id = $1
    AND parent_ticket_id = $2
    AND operation_type = 'POWER';

-- name: ResetDomainEventForRetry :execrows
UPDATE domain_events
SET status = 'PENDING'
WHERE id = $1;

-- name: InsertDomainEvent :exec
INSERT INTO domain_events (
    id,
    created_at,
    event_type,
    aggregate_type,
    aggregate_id,
    payload,
    status,
    created_by
) VALUES (
    $1,
    NOW(),
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
);

-- name: InsertTicket :exec
INSERT INTO tickets (
    id,
    created_at,
    updated_at,
    event_id,
    operation_type,
    status,
    requester,
    reason,
    parent_ticket_id
) VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3,
    $4,
    $5,
    sqlc.narg(reason),
    sqlc.narg(parent_ticket_id)
);

-- name: InsertBatchTicket :exec
INSERT INTO batch_tickets (
    id,
    created_at,
    updated_at,
    batch_type,
    child_count,
    pending_count,
    status,
    request_id,
    created_by,
    reason
) VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3,
    $4,
    $5,
    sqlc.narg(request_id),
    $6,
    sqlc.narg(reason)
);

-- name: AllocateServiceInstance :one
WITH allocated AS (
    UPDATE services AS s
    SET
        next_instance_index = s.next_instance_index + 1,
        updated_at = NOW()
    WHERE s.id = $1
    RETURNING s.id, s.name, s.system_services, s.next_instance_index - 1 AS allocated_index
)
SELECT
    allocated.id AS service_id,
    allocated.name AS service_name,
    systems.name AS system_name,
    allocated.allocated_index
FROM allocated
JOIN systems ON systems.id = allocated.system_services;

-- name: InsertVM :exec
INSERT INTO vms (
    id,
    created_at,
    updated_at,
    name,
    instance,
    namespace,
    cluster_id,
    status,
    hostname,
    created_by,
    ticket_id,
    root_volume_storage_class,
    root_volume_access_modes,
    root_volume_volume_mode,
    service_vms
) VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    $3,
    $4,
    $5,
    'CREATING',
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12
);

-- name: SetVMStatus :execrows
UPDATE vms
SET
    status = $2,
    updated_at = NOW()
WHERE id = $1;
