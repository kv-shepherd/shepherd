import { describe, expect, it } from 'vitest';

import {
    buildBatchResultExportFilename,
    buildBatchResultExportPayload,
    canExportBatchResult,
    type VMBatchStatus,
} from './batchExport';

const sampleBatch: VMBatchStatus = {
    batch_id: 'batch/needs spaces',
    operation: 'DELETE',
    status: 'PARTIAL_SUCCESS',
    child_count: 2,
    success_count: 1,
    failed_count: 1,
    pending_count: 0,
    created_by: 'owner-1',
    created_at: '2026-03-17T00:00:00Z',
    updated_at: '2026-03-17T00:05:00Z',
    children: [
        {
            ticket_id: 'ticket-1',
            event_id: 'event-1',
            status: 'SUCCESS',
            resource_id: 'vm-1',
            resource_name: 'vm-a',
            attempt_count: 1,
        },
        {
            ticket_id: 'ticket-2',
            event_id: 'event-2',
            status: 'FAILED',
            resource_id: 'vm-2',
            resource_name: 'vm-b',
            last_error: 'quota exceeded',
        },
    ],
};

describe('batchExport', () => {
    it('allows export only for result-bearing terminal batch statuses', () => {
        expect(canExportBatchResult('COMPLETED')).toBe(true);
        expect(canExportBatchResult('PARTIAL_SUCCESS')).toBe(true);
        expect(canExportBatchResult('FAILED')).toBe(true);
        expect(canExportBatchResult('CANCELLED')).toBe(false);
        expect(canExportBatchResult('IN_PROGRESS')).toBe(false);
        expect(canExportBatchResult(undefined)).toBe(false);
    });

    it('builds a stable export payload from the canonical batch detail response', () => {
        const payload = buildBatchResultExportPayload(
            sampleBatch,
            new Date('2026-03-17T00:06:00Z'),
        );

        expect(payload).toMatchObject({
            schema_version: 'vm-batch-result-export/v1',
            exported_at: '2026-03-17T00:06:00.000Z',
            batch: {
                batch_id: 'batch/needs spaces',
                operation: 'DELETE',
                status: 'PARTIAL_SUCCESS',
                child_count: 2,
                success_count: 1,
                failed_count: 1,
                pending_count: 0,
                created_by: 'owner-1',
            },
            children: [
                {
                    ticket_id: 'ticket-1',
                    event_id: 'event-1',
                    status: 'SUCCESS',
                    resource_name: 'vm-a',
                    attempt_count: 1,
                },
                {
                    ticket_id: 'ticket-2',
                    event_id: 'event-2',
                    status: 'FAILED',
                    resource_name: 'vm-b',
                    attempt_count: 0,
                    last_error: 'quota exceeded',
                },
            ],
        });
    });

    it('sanitizes export filenames', () => {
        expect(buildBatchResultExportFilename(
            'batch/needs spaces',
            new Date('2026-03-17T00:06:00Z'),
        )).toBe('vm-batch-batch-needs-spaces-2026-03-17T00-06-00-000Z.json');
    });
});
