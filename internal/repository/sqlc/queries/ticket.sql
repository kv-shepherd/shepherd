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
    attempt_count = CASE
        WHEN parent_ticket_id IS NOT NULL AND attempt_count = 0 THEN 1
        ELSE attempt_count
    END,
    last_attempt_at = CASE
        WHEN parent_ticket_id IS NOT NULL AND attempt_count = 0 THEN NOW()
        ELSE last_attempt_at
    END,
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
    attempt_count = CASE
        WHEN parent_ticket_id IS NOT NULL AND attempt_count = 0 THEN 1
        ELSE attempt_count
    END,
    last_attempt_at = CASE
        WHEN parent_ticket_id IS NOT NULL AND attempt_count = 0 THEN NOW()
        ELSE last_attempt_at
    END,
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
    attempt_count = CASE
        WHEN parent_ticket_id IS NOT NULL AND attempt_count = 0 THEN 1
        ELSE attempt_count
    END,
    last_attempt_at = CASE
        WHEN parent_ticket_id IS NOT NULL AND attempt_count = 0 THEN NOW()
        ELSE last_attempt_at
    END,
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
    attempt_count = CASE
        WHEN parent_ticket_id IS NOT NULL AND attempt_count = 0 THEN 1
        ELSE attempt_count
    END,
    last_attempt_at = CASE
        WHEN parent_ticket_id IS NOT NULL AND attempt_count = 0 THEN NOW()
        ELSE last_attempt_at
    END,
    updated_at = NOW()
WHERE
    id = sqlc.arg(id)
    AND event_id = sqlc.arg(event_id)
    AND status = 'PENDING'
    AND operation_type = 'POWER';

-- name: SetDomainEventStatus :execrows
UPDATE domain_events
SET status = $2
WHERE id = $1 AND status = 'PENDING';

-- name: ClaimBatchApprovalDispatch :execrows
UPDATE tickets AS parent
SET
    status = 'EXECUTING',
    approver = sqlc.arg(approver),
    selected_cluster_id = sqlc.arg(selected_cluster_id),
    selected_storage_class = sqlc.arg(selected_storage_class),
    modified_spec = COALESCE(modified_spec, '{}'::jsonb) ||
        jsonb_build_object('batch_approval_execution', sqlc.arg(execution_options)::jsonb),
    reject_reason = NULL,
    updated_at = NOW()
WHERE parent.id = sqlc.arg(id)
  AND parent.event_id = sqlc.arg(event_id)
  AND parent.parent_ticket_id IS NULL
  AND parent.operation_type IN ('CREATE', 'MODIFY', 'DELETE', 'POWER')
  AND parent.status = 'PENDING'
  AND NULLIF(BTRIM(parent.requester), '') IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM domain_events AS event
      WHERE event.id = sqlc.arg(event_id)
        AND event.aggregate_type = 'batch'
        AND event.aggregate_id = sqlc.arg(id)
        AND event.status = 'PENDING'
        AND event.created_by = parent.requester
        AND (
            (parent.operation_type = 'CREATE' AND event.event_type = 'BATCH_CREATE_REQUESTED')
            OR (parent.operation_type = 'MODIFY' AND event.event_type = 'BATCH_MODIFY_REQUESTED')
            OR (parent.operation_type = 'DELETE' AND event.event_type = 'BATCH_DELETE_REQUESTED')
            OR (parent.operation_type = 'POWER' AND event.event_type = 'BATCH_POWER_REQUESTED')
        )
  )
  AND EXISTS (
      SELECT 1
      FROM batch_tickets AS batch
      WHERE batch.id = parent.id
        AND batch.created_by = parent.requester
        AND batch.status = 'PENDING_APPROVAL'
        AND batch.batch_type::text = CASE parent.operation_type::text
            WHEN 'CREATE' THEN 'BATCH_CREATE'
            WHEN 'MODIFY' THEN 'BATCH_MODIFY'
            WHEN 'DELETE' THEN 'BATCH_DELETE'
            WHEN 'POWER' THEN 'BATCH_POWER'
        END
  );

-- name: ReopenBatchApprovalDispatch :execrows
UPDATE tickets AS parent
SET
    status = 'EXECUTING',
    approver = sqlc.arg(approver),
    selected_cluster_id = sqlc.arg(selected_cluster_id),
    selected_storage_class = sqlc.arg(selected_storage_class),
    modified_spec = COALESCE(modified_spec, '{}'::jsonb) ||
        jsonb_build_object('batch_approval_execution', sqlc.arg(execution_options)::jsonb),
    reject_reason = NULL,
    updated_at = NOW()
WHERE parent.id = sqlc.arg(id)
  AND parent.event_id = sqlc.arg(event_id)
  AND parent.parent_ticket_id IS NULL
  AND parent.operation_type IN ('CREATE', 'MODIFY', 'DELETE')
  AND parent.status IN ('EXECUTING', 'FAILED')
  AND NULLIF(BTRIM(parent.approver), '') IS NOT NULL
  AND NULLIF(BTRIM(parent.requester), '') IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM domain_events AS event
      WHERE event.id = sqlc.arg(event_id)
        AND event.aggregate_type = 'batch'
        AND event.aggregate_id = sqlc.arg(id)
        AND event.status IN ('PROCESSING', 'FAILED')
        AND event.created_by = parent.requester
        AND (
            (parent.operation_type = 'CREATE' AND event.event_type = 'BATCH_CREATE_REQUESTED')
            OR (parent.operation_type = 'MODIFY' AND event.event_type = 'BATCH_MODIFY_REQUESTED')
            OR (parent.operation_type = 'DELETE' AND event.event_type = 'BATCH_DELETE_REQUESTED')
        )
  )
  AND EXISTS (
      SELECT 1
      FROM batch_tickets AS batch
      WHERE batch.id = parent.id
        AND batch.created_by = parent.requester
        AND (
            (parent.status = 'EXECUTING' AND batch.status = 'IN_PROGRESS')
            OR (parent.status = 'FAILED' AND batch.status IN ('FAILED', 'PARTIAL_SUCCESS'))
        )
        AND batch.batch_type::text = CASE parent.operation_type::text
            WHEN 'CREATE' THEN 'BATCH_CREATE'
            WHEN 'MODIFY' THEN 'BATCH_MODIFY'
            WHEN 'DELETE' THEN 'BATCH_DELETE'
        END
  );

-- name: SetBatchApprovalEventProcessing :execrows
UPDATE domain_events AS event
SET status = 'PROCESSING'
FROM tickets AS parent
WHERE event.id = sqlc.arg(event_id)
  AND parent.id = sqlc.arg(parent_id)
  AND parent.event_id = event.id
  AND parent.parent_ticket_id IS NULL
  AND NULLIF(BTRIM(parent.requester), '') IS NOT NULL
  AND event.aggregate_type = 'batch'
  AND event.aggregate_id = parent.id
  AND event.status IN ('PROCESSING', 'FAILED')
  AND event.created_by = parent.requester
  AND (
      (parent.operation_type = 'CREATE' AND event.event_type = 'BATCH_CREATE_REQUESTED')
      OR (parent.operation_type = 'MODIFY' AND event.event_type = 'BATCH_MODIFY_REQUESTED')
      OR (parent.operation_type = 'DELETE' AND event.event_type = 'BATCH_DELETE_REQUESTED')
  )
  AND EXISTS (
      SELECT 1
      FROM batch_tickets AS batch
      WHERE batch.id = parent.id
        AND batch.created_by = parent.requester
        AND batch.status IN ('IN_PROGRESS', 'FAILED', 'PARTIAL_SUCCESS')
        AND batch.batch_type::text = CASE parent.operation_type::text
            WHEN 'CREATE' THEN 'BATCH_CREATE'
            WHEN 'MODIFY' THEN 'BATCH_MODIFY'
            WHEN 'DELETE' THEN 'BATCH_DELETE'
        END
  );

-- name: ClaimBatchApprovalEventProcessing :execrows
UPDATE domain_events AS event
SET status = 'PROCESSING'
FROM tickets AS parent
WHERE event.id = sqlc.arg(event_id)
  AND parent.id = sqlc.arg(parent_id)
  AND parent.event_id = event.id
  AND parent.parent_ticket_id IS NULL
  AND NULLIF(BTRIM(parent.requester), '') IS NOT NULL
  AND event.aggregate_type = 'batch'
  AND event.aggregate_id = parent.id
  AND event.status = 'PENDING'
  AND event.created_by = parent.requester
  AND (
      (parent.operation_type = 'CREATE' AND event.event_type = 'BATCH_CREATE_REQUESTED')
      OR (parent.operation_type = 'MODIFY' AND event.event_type = 'BATCH_MODIFY_REQUESTED')
      OR (parent.operation_type = 'DELETE' AND event.event_type = 'BATCH_DELETE_REQUESTED')
      OR (parent.operation_type = 'POWER' AND event.event_type = 'BATCH_POWER_REQUESTED')
  )
  AND EXISTS (
      SELECT 1
      FROM batch_tickets AS batch
      WHERE batch.id = parent.id
        AND batch.created_by = parent.requester
        AND batch.status = 'PENDING_APPROVAL'
        AND batch.batch_type::text = CASE parent.operation_type::text
            WHEN 'CREATE' THEN 'BATCH_CREATE'
            WHEN 'MODIFY' THEN 'BATCH_MODIFY'
            WHEN 'DELETE' THEN 'BATCH_DELETE'
            WHEN 'POWER' THEN 'BATCH_POWER'
        END
  );

-- name: ResetBatchApprovalRetryChild :execrows
UPDATE tickets AS child
SET
    status = 'PENDING',
    reject_reason = NULL,
    attempt_count = attempt_count + 1,
    last_attempt_at = NOW(),
    updated_at = NOW()
WHERE child.id = sqlc.arg(id)
  AND child.event_id = sqlc.arg(event_id)
  AND child.parent_ticket_id = sqlc.arg(parent_ticket_id)
  AND child.status = 'FAILED'
  AND child.operation_type <> 'POWER'
  AND NULLIF(BTRIM(child.requester), '') IS NOT NULL
  AND child.attempt_count >= 0
  AND child.attempt_count < sqlc.arg(max_attempts)
  AND EXISTS (
      SELECT 1
      FROM domain_events AS event
      WHERE event.id = sqlc.arg(event_id)
        AND event.aggregate_type = 'vm'
        AND event.status IN ('PENDING', 'FAILED', 'CANCELLED')
        AND event.created_by = child.requester
        AND (
            (child.operation_type = 'CREATE' AND event.event_type = 'VM_CREATION_REQUESTED')
            OR (child.operation_type = 'MODIFY' AND event.event_type = 'VM_MODIFY_REQUESTED')
            OR (child.operation_type = 'DELETE' AND event.event_type = 'VM_DELETION_REQUESTED')
        )
  )
  AND EXISTS (
      SELECT 1
      FROM tickets AS parent
      JOIN domain_events AS parent_event ON parent_event.id = parent.event_id
      JOIN batch_tickets AS batch ON batch.id = parent.id
      WHERE parent.id = child.parent_ticket_id
        AND parent.parent_ticket_id IS NULL
        AND parent.operation_type = child.operation_type
        AND parent.requester = child.requester
        AND parent.status IN ('EXECUTING', 'FAILED')
        AND parent_event.aggregate_type = 'batch'
        AND parent_event.aggregate_id = parent.id
        AND parent_event.created_by = parent.requester
        AND parent_event.status IN ('PROCESSING', 'FAILED')
        AND (
            (parent.operation_type = 'CREATE' AND parent_event.event_type = 'BATCH_CREATE_REQUESTED')
            OR (parent.operation_type = 'MODIFY' AND parent_event.event_type = 'BATCH_MODIFY_REQUESTED')
            OR (parent.operation_type = 'DELETE' AND parent_event.event_type = 'BATCH_DELETE_REQUESTED')
        )
        AND batch.created_by = parent.requester
        AND (
            (parent.status = 'EXECUTING' AND batch.status = 'IN_PROGRESS')
            OR (parent.status = 'FAILED' AND batch.status IN ('FAILED', 'PARTIAL_SUCCESS'))
        )
        AND batch.batch_type::text = CASE parent.operation_type::text
            WHEN 'CREATE' THEN 'BATCH_CREATE'
            WHEN 'MODIFY' THEN 'BATCH_MODIFY'
            WHEN 'DELETE' THEN 'BATCH_DELETE'
        END
  );

-- name: ResetBatchApprovalRetryEvent :execrows
UPDATE domain_events AS event
SET status = 'PENDING'
FROM tickets AS child
WHERE event.id = sqlc.arg(event_id)
  AND child.id = sqlc.arg(ticket_id)
  AND child.event_id = event.id
  AND child.parent_ticket_id = sqlc.arg(parent_ticket_id)
  AND child.status = 'PENDING'
  AND child.operation_type <> 'POWER'
  AND NULLIF(BTRIM(child.requester), '') IS NOT NULL
  AND event.aggregate_type = 'vm'
  AND event.status IN ('PENDING', 'FAILED', 'CANCELLED')
  AND event.created_by = child.requester
  AND (
      (child.operation_type = 'CREATE' AND event.event_type = 'VM_CREATION_REQUESTED')
      OR (child.operation_type = 'MODIFY' AND event.event_type = 'VM_MODIFY_REQUESTED')
      OR (child.operation_type = 'DELETE' AND event.event_type = 'VM_DELETION_REQUESTED')
  )
  AND EXISTS (
      SELECT 1
      FROM tickets AS parent
      JOIN domain_events AS parent_event ON parent_event.id = parent.event_id
      JOIN batch_tickets AS batch ON batch.id = parent.id
      WHERE parent.id = child.parent_ticket_id
        AND parent.parent_ticket_id IS NULL
        AND parent.operation_type = child.operation_type
        AND parent.requester = child.requester
        AND parent.status IN ('EXECUTING', 'FAILED')
        AND parent_event.aggregate_type = 'batch'
        AND parent_event.aggregate_id = parent.id
        AND parent_event.created_by = parent.requester
        AND parent_event.status IN ('PROCESSING', 'FAILED')
        AND (
            (parent.operation_type = 'CREATE' AND parent_event.event_type = 'BATCH_CREATE_REQUESTED')
            OR (parent.operation_type = 'MODIFY' AND parent_event.event_type = 'BATCH_MODIFY_REQUESTED')
            OR (parent.operation_type = 'DELETE' AND parent_event.event_type = 'BATCH_DELETE_REQUESTED')
        )
        AND batch.created_by = parent.requester
        AND (
            (parent.status = 'EXECUTING' AND batch.status = 'IN_PROGRESS')
            OR (parent.status = 'FAILED' AND batch.status IN ('FAILED', 'PARTIAL_SUCCESS'))
        )
        AND batch.batch_type::text = CASE parent.operation_type::text
            WHEN 'CREATE' THEN 'BATCH_CREATE'
            WHEN 'MODIFY' THEN 'BATCH_MODIFY'
            WHEN 'DELETE' THEN 'BATCH_DELETE'
        END
  );

-- name: RefreshBatchApprovalProjectionForDispatch :execrows
WITH parent_identity AS (
    SELECT parent.id, parent.operation_type, parent.requester
    FROM tickets AS parent
    JOIN domain_events AS event ON event.id = parent.event_id
    WHERE parent.id = sqlc.arg(parent_id)
      AND parent.parent_ticket_id IS NULL
      AND parent.operation_type IN ('CREATE', 'MODIFY', 'DELETE', 'POWER')
      AND parent.status = 'EXECUTING'
      AND NULLIF(BTRIM(parent.requester), '') IS NOT NULL
      AND event.aggregate_type = 'batch'
      AND event.aggregate_id = parent.id
      AND event.created_by = parent.requester
      AND event.status = 'PROCESSING'
      AND (
          (parent.operation_type = 'CREATE' AND event.event_type = 'BATCH_CREATE_REQUESTED')
          OR (parent.operation_type = 'MODIFY' AND event.event_type = 'BATCH_MODIFY_REQUESTED')
          OR (parent.operation_type = 'DELETE' AND event.event_type = 'BATCH_DELETE_REQUESTED')
          OR (parent.operation_type = 'POWER' AND event.event_type = 'BATCH_POWER_REQUESTED')
      )
),
child_counts AS (
    SELECT
        parent.id AS parent_id,
        COUNT(*)::integer AS child_count,
        COUNT(*) FILTER (WHERE status = 'SUCCESS')::integer AS success_count,
        COUNT(*) FILTER (WHERE status IN ('FAILED', 'REJECTED'))::integer AS failed_count,
        COUNT(*) FILTER (
            WHERE status NOT IN ('SUCCESS', 'FAILED', 'REJECTED', 'CANCELLED')
        )::integer AS pending_count
    FROM parent_identity AS parent
    JOIN tickets AS child ON child.parent_ticket_id = parent.id
    GROUP BY parent.id
)
UPDATE batch_tickets AS batch
SET
    child_count = child_counts.child_count,
    success_count = child_counts.success_count,
    failed_count = child_counts.failed_count,
    pending_count = child_counts.pending_count,
    status = 'IN_PROGRESS',
    updated_at = NOW()
FROM child_counts
JOIN parent_identity AS parent ON parent.id = child_counts.parent_id
WHERE batch.id = parent.id
  AND batch.created_by = parent.requester
  AND batch.status IN ('PENDING_APPROVAL', 'IN_PROGRESS', 'FAILED', 'PARTIAL_SUCCESS')
  AND batch.batch_type::text = CASE parent.operation_type::text
      WHEN 'CREATE' THEN 'BATCH_CREATE'
      WHEN 'MODIFY' THEN 'BATCH_MODIFY'
      WHEN 'DELETE' THEN 'BATCH_DELETE'
      WHEN 'POWER' THEN 'BATCH_POWER'
  END
  AND child_counts.child_count > 0;

-- name: ResetPowerRetryTicket :execrows
UPDATE tickets AS child
SET
    status = 'EXECUTING',
    reject_reason = NULL,
    attempt_count = attempt_count + 1,
    last_attempt_at = NOW(),
    updated_at = NOW()
WHERE child.id = sqlc.arg(id)
  AND child.event_id = sqlc.arg(event_id)
  AND child.parent_ticket_id = sqlc.arg(parent_ticket_id)
  AND child.status = 'FAILED'
  AND child.operation_type = 'POWER'
  AND NULLIF(BTRIM(child.requester), '') IS NOT NULL
  AND child.attempt_count >= 0
  AND child.attempt_count < sqlc.arg(max_attempts)
  AND EXISTS (
      SELECT 1
      FROM domain_events AS event
      WHERE event.id = child.event_id
        AND event.aggregate_type = 'vm'
        AND NULLIF(BTRIM(event.aggregate_id), '') IS NOT NULL
        AND event.event_type IN ('VM_START_REQUESTED', 'VM_STOP_REQUESTED', 'VM_RESTART_REQUESTED')
        AND event.status IN ('FAILED', 'CANCELLED')
        AND event.created_by = child.requester
  )
  AND EXISTS (
      SELECT 1
      FROM tickets AS parent
      JOIN domain_events AS parent_event ON parent_event.id = parent.event_id
      JOIN batch_tickets AS batch ON batch.id = parent.id
      WHERE parent.id = child.parent_ticket_id
        AND parent.parent_ticket_id IS NULL
        AND parent.operation_type = 'POWER'
        AND parent.requester = child.requester
        AND parent.status IN ('EXECUTING', 'FAILED')
        AND parent_event.aggregate_type = 'batch'
        AND parent_event.aggregate_id = parent.id
        AND parent_event.event_type = 'BATCH_POWER_REQUESTED'
        AND parent_event.status IN ('PROCESSING', 'FAILED')
        AND parent_event.created_by = parent.requester
        AND batch.batch_type = 'BATCH_POWER'
        AND batch.created_by = parent.requester
        AND (
            (parent.status = 'EXECUTING' AND batch.status = 'IN_PROGRESS')
            OR (parent.status = 'FAILED' AND batch.status IN ('FAILED', 'PARTIAL_SUCCESS'))
        )
  );

-- name: StartInitialBatchChildAttempt :execrows
UPDATE tickets
SET
    attempt_count = 1,
    last_attempt_at = NOW(),
    updated_at = NOW()
WHERE
    id = sqlc.arg(id)
    AND event_id = sqlc.arg(event_id)
    AND parent_ticket_id = sqlc.arg(parent_ticket_id)
    AND attempt_count = 0;

-- name: ResetBatchPowerRetryEvent :execrows
UPDATE domain_events AS event
SET status = 'PENDING'
FROM tickets AS child
WHERE event.id = sqlc.arg(event_id)
  AND child.id = sqlc.arg(ticket_id)
  AND child.event_id = event.id
  AND child.parent_ticket_id = sqlc.arg(parent_ticket_id)
  AND child.operation_type = 'POWER'
  AND child.status = 'EXECUTING'
  AND NULLIF(BTRIM(child.requester), '') IS NOT NULL
  AND event.aggregate_type = 'vm'
  AND NULLIF(BTRIM(event.aggregate_id), '') IS NOT NULL
  AND event.event_type IN ('VM_START_REQUESTED', 'VM_STOP_REQUESTED', 'VM_RESTART_REQUESTED')
  AND event.status IN ('FAILED', 'CANCELLED')
  AND event.created_by = child.requester
  AND EXISTS (
      SELECT 1
      FROM tickets AS parent
      JOIN domain_events AS parent_event ON parent_event.id = parent.event_id
      JOIN batch_tickets AS batch ON batch.id = parent.id
      WHERE parent.id = child.parent_ticket_id
        AND parent.parent_ticket_id IS NULL
        AND parent.operation_type = 'POWER'
        AND parent.requester = child.requester
        AND parent.status IN ('EXECUTING', 'FAILED')
        AND parent_event.aggregate_type = 'batch'
        AND parent_event.aggregate_id = parent.id
        AND parent_event.event_type = 'BATCH_POWER_REQUESTED'
        AND parent_event.status IN ('PROCESSING', 'FAILED')
        AND parent_event.created_by = parent.requester
        AND batch.batch_type = 'BATCH_POWER'
        AND batch.created_by = parent.requester
        AND (
            (parent.status = 'EXECUTING' AND batch.status = 'IN_PROGRESS')
            OR (parent.status = 'FAILED' AND batch.status IN ('FAILED', 'PARTIAL_SUCCESS'))
        )
  );

-- name: ReopenBatchPowerParentForRetry :one
WITH parent_update AS (
    UPDATE tickets AS parent
    SET
        status = 'EXECUTING',
        updated_at = NOW()
    WHERE parent.id = sqlc.arg(parent_id)
      AND parent.parent_ticket_id IS NULL
      AND parent.operation_type = 'POWER'
      AND parent.status IN ('EXECUTING', 'FAILED')
      AND NULLIF(BTRIM(parent.requester), '') IS NOT NULL
      AND EXISTS (
          SELECT 1
          FROM domain_events AS event
          WHERE event.id = parent.event_id
            AND event.aggregate_type = 'batch'
            AND event.aggregate_id = parent.id
            AND event.event_type = 'BATCH_POWER_REQUESTED'
            AND event.status IN ('PROCESSING', 'FAILED')
            AND event.created_by = parent.requester
      )
      AND EXISTS (
          SELECT 1
          FROM batch_tickets AS batch
          WHERE batch.id = parent.id
            AND batch.batch_type = 'BATCH_POWER'
            AND batch.created_by = parent.requester
            AND (
                (parent.status = 'EXECUTING' AND batch.status = 'IN_PROGRESS')
                OR (parent.status = 'FAILED' AND batch.status IN ('FAILED', 'PARTIAL_SUCCESS'))
            )
      )
    RETURNING parent.event_id, parent.requester
),
event_update AS (
    UPDATE domain_events AS event
    SET status = 'PROCESSING'
    FROM parent_update
    WHERE event.id = parent_update.event_id
      AND event.aggregate_type = 'batch'
      AND event.aggregate_id = sqlc.arg(parent_id)
      AND event.event_type = 'BATCH_POWER_REQUESTED'
      AND event.status IN ('PROCESSING', 'FAILED')
      AND event.created_by = parent_update.requester
    RETURNING event.id
)
SELECT id FROM event_update;

-- name: RefreshBatchPowerProjectionForRetry :execrows
WITH parent_identity AS (
    SELECT parent.id, parent.requester
    FROM tickets AS parent
    JOIN domain_events AS event ON event.id = parent.event_id
    WHERE parent.id = sqlc.arg(parent_id)
      AND parent.parent_ticket_id IS NULL
      AND parent.operation_type = 'POWER'
      AND parent.status = 'EXECUTING'
      AND NULLIF(BTRIM(parent.requester), '') IS NOT NULL
      AND event.aggregate_type = 'batch'
      AND event.aggregate_id = parent.id
      AND event.event_type = 'BATCH_POWER_REQUESTED'
      AND event.status = 'PROCESSING'
      AND event.created_by = parent.requester
),
child_counts AS (
    SELECT
        parent.id AS parent_id,
        COUNT(*)::integer AS child_count,
        COUNT(*) FILTER (WHERE child.status = 'SUCCESS')::integer AS success_count,
        COUNT(*) FILTER (WHERE child.status IN ('FAILED', 'REJECTED'))::integer AS failed_count,
        COUNT(*) FILTER (
            WHERE child.status NOT IN ('SUCCESS', 'FAILED', 'REJECTED', 'CANCELLED')
        )::integer AS pending_count
    FROM parent_identity AS parent
    JOIN tickets AS child ON child.parent_ticket_id = parent.id
    GROUP BY parent.id
)
UPDATE batch_tickets AS batch
SET
    child_count = child_counts.child_count,
    success_count = child_counts.success_count,
    failed_count = child_counts.failed_count,
    pending_count = child_counts.pending_count,
    status = 'IN_PROGRESS',
    updated_at = NOW()
FROM child_counts
JOIN parent_identity AS parent ON parent.id = child_counts.parent_id
WHERE batch.id = parent.id
  AND batch.batch_type = 'BATCH_POWER'
  AND batch.created_by = parent.requester
  AND batch.status IN ('IN_PROGRESS', 'FAILED', 'PARTIAL_SUCCESS')
  AND child_counts.child_count > 0;

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
