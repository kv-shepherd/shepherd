-- name: ApproveCreateTicket :execrows
UPDATE approval_tickets
SET
    status = 'APPROVED',
    approver = sqlc.arg(approver),
    selected_cluster_id = sqlc.arg(selected_cluster_id),
    selected_template_version = CASE
        WHEN sqlc.narg(selected_template_version)::int IS NULL THEN selected_template_version
        ELSE sqlc.narg(selected_template_version)::int
    END,
    selected_storage_class = CASE
        WHEN sqlc.arg(selected_storage_class)::text = '' THEN selected_storage_class
        ELSE sqlc.arg(selected_storage_class)::text
    END,
    template_snapshot = COALESCE(sqlc.narg(template_snapshot)::jsonb, template_snapshot),
    instance_size_snapshot = COALESCE(sqlc.narg(instance_size_snapshot)::jsonb, instance_size_snapshot),
    modified_spec = COALESCE(sqlc.narg(modified_spec)::jsonb, modified_spec),
    updated_at = NOW()
WHERE
    id = sqlc.arg(id)
    AND event_id = sqlc.arg(event_id)
    AND status = 'PENDING'
    AND operation_type = 'CREATE';

-- name: ApproveDeleteTicket :execrows
UPDATE approval_tickets
SET
    status = 'APPROVED',
    approver = sqlc.arg(approver),
    updated_at = NOW()
WHERE
    id = sqlc.arg(id)
    AND event_id = sqlc.arg(event_id)
    AND status = 'PENDING'
    AND operation_type = 'DELETE';

-- name: SetDomainEventStatus :execrows
UPDATE domain_events
SET status = $2
WHERE id = $1;

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
    $9
);

-- name: SetVMStatus :execrows
UPDATE vms
SET
    status = $2,
    updated_at = NOW()
WHERE id = $1;
