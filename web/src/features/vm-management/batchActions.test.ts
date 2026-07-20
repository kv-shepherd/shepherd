import { describe, expect, it } from 'vitest';

import {
    canManageBatchOperation,
    createBatchActionFeedback,
    createCanonicalBatchIntentFingerprint,
    extractBatchRetryAfterSeconds,
    extractRestartReconciliationNotice,
    isBatchConflictError,
    isRetryableBatchChild,
} from './batchActions';

const child = (status: 'FAILED' | 'REJECTED', attempt_count?: number) => ({
    ticket_id: `ticket-${status}-${attempt_count ?? 'omitted'}`,
    event_id: 'event-1',
    status,
    ...(attempt_count === undefined ? {} : { attempt_count }),
});

describe('batch action safety helpers', () => {
    it('canonicalizes key and item order while distinguishing changed batch intent', () => {
        const first = createCanonicalBatchIntentFingerprint({
            operation: 'RESTART',
            items: [{ vm_id: 'vm-2' }, { vm_id: 'vm-1' }],
        });
        const reordered = createCanonicalBatchIntentFingerprint({
            items: [{ vm_id: 'vm-1' }, { vm_id: 'vm-2' }],
            operation: 'RESTART',
        });
        const changed = createCanonicalBatchIntentFingerprint({
            operation: 'RESTART',
            items: [{ vm_id: 'vm-3' }],
        });

        expect(reordered).toBe(first);
        expect(changed).not.toBe(first);
    });

    it('treats omitted attempt_count as zero and retries only non-exhausted FAILED children', () => {
        expect(isRetryableBatchChild(child('FAILED'))).toBe(true);
        expect(isRetryableBatchChild(child('FAILED', 2))).toBe(true);
        expect(isRetryableBatchChild(child('FAILED', 3))).toBe(false);
        expect(isRetryableBatchChild(child('REJECTED', 0))).toBe(false);
    });

    it.each([
        ['CREATE', 'vm:create'],
        ['DELETE', 'vm:delete'],
        ['MODIFY', 'vm:operate'],
        ['POWER', 'vm:operate'],
        ['RESTART', 'vm:operate'],
    ])('requires the operation-specific permission for %s', (operation, permission) => {
        expect(canManageBatchOperation({
            id: 'user-1',
            username: 'operator',
            permissions: [permission],
        }, operation)).toBe(true);
        expect(canManageBatchOperation({
            id: 'user-2',
            username: 'viewer',
            permissions: ['vm:read'],
        }, operation)).toBe(false);
    });

    it('allows builtin approvers and platform admins to manage every known operation', () => {
        for (const permission of ['builtin_approval:approve', 'platform:admin']) {
            expect(canManageBatchOperation({
                id: permission,
                username: permission,
                permissions: [permission],
            }, 'DELETE')).toBe(true);
        }
        expect(canManageBatchOperation({
            id: 'unknown',
            username: 'unknown',
            permissions: ['builtin_approval:approve'],
        }, 'UNSUPPORTED')).toBe(false);
    });

    it('uses authoritative affected ids and falls back only up to affected_count', () => {
        expect(createBatchActionFeedback('retry', {
            batch_id: 'batch-1',
            status: 'IN_PROGRESS',
            affected_count: 2,
            affected_ticket_ids: [' ticket-2 ', 'ticket-1', 'ticket-2'],
        }, ['fallback-1', 'fallback-2'])).toEqual({
            action: 'retry',
            affectedCount: 2,
            affectedTicketIDs: ['ticket-2', 'ticket-1'],
        });
        expect(createBatchActionFeedback('cancel', {
            batch_id: 'batch-1',
            status: 'CANCELLED',
            affected_count: 1,
        }, ['pending-1', 'pending-2'])).toEqual({
            action: 'cancel',
            affectedCount: 1,
            affectedTicketIDs: ['pending-1'],
        });
    });

    it('normalizes Retry-After, conflict state, and strict restart recovery metadata', () => {
        expect(extractBatchRetryAfterSeconds({
            code: 'BATCH_RATE_LIMITED',
            retry_after_seconds: 2.1,
        })).toBe(3);
        expect(extractBatchRetryAfterSeconds({
            code: 'BATCH_RATE_LIMITED',
            params: { retry_after_seconds: '4' },
        })).toBe(4);
        expect(isBatchConflictError({ code: 'STATE_CONFLICT' })).toBe(true);
        expect(isBatchConflictError({ code: 'BATCH_ACTION_NOT_APPLICABLE', status: 409 })).toBe(true);
        expect(extractRestartReconciliationNotice({
            code: 'POWER_OPERATION_IN_PROGRESS',
            params: {
                operator_action_required: true,
                existing_event_id: ' event-1 ',
                reconciliation_path: ' operator-runbook:ambiguous-vm-restart ',
            },
        })).toEqual({
            eventId: 'event-1',
            reconciliationPath: 'operator-runbook:ambiguous-vm-restart',
        });
        expect(extractRestartReconciliationNotice({
            code: 'POWER_OPERATION_IN_PROGRESS',
            params: { operator_action_required: true },
        })).toBeNull();
        expect(extractRestartReconciliationNotice({
            code: 'POWER_OPERATION_IN_PROGRESS',
            params: {
                operator_action_required: true,
                existing_event_id: 'event-1',
                reconciliation_path: 'operator-runbook:unsupported',
            },
        })).toBeNull();
    });
});
