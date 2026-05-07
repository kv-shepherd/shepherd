import type { components } from '@/types/api.gen';

export type VMBatchStatus = components['schemas']['VMBatchStatusResponse'];

const EXPORTABLE_BATCH_STATUSES = new Set<string>([
    'COMPLETED',
    'PARTIAL_SUCCESS',
    'FAILED',
]);

interface VMBatchResultExportPayload {
    schema_version: 'vm-batch-result-export/v1';
    exported_at: string;
    batch: {
        batch_id: string;
        operation: string;
        status: string;
        child_count: number;
        success_count: number;
        failed_count: number;
        pending_count: number;
        created_by: string;
        created_at: string;
        updated_at: string;
    };
    children: Array<{
        ticket_id: string;
        event_id: string;
        status: string;
        resource_id?: string;
        resource_name?: string;
        attempt_count: number;
        last_error?: string;
    }>;
}

export function canExportBatchResult(status: string | undefined): boolean {
    return typeof status === 'string' && EXPORTABLE_BATCH_STATUSES.has(status);
}

export function buildBatchResultExportPayload(
    batch: VMBatchStatus,
    exportedAt: Date = new Date(),
): VMBatchResultExportPayload {
    return {
        schema_version: 'vm-batch-result-export/v1',
        exported_at: exportedAt.toISOString(),
        batch: {
            batch_id: batch.batch_id,
            operation: batch.operation,
            status: batch.status,
            child_count: batch.child_count,
            success_count: batch.success_count,
            failed_count: batch.failed_count,
            pending_count: batch.pending_count,
            created_by: batch.created_by,
            created_at: batch.created_at,
            updated_at: batch.updated_at,
        },
        children: batch.children.map((child) => ({
            ticket_id: child.ticket_id,
            event_id: child.event_id,
            status: child.status,
            resource_id: child.resource_id,
            resource_name: child.resource_name,
            attempt_count: child.attempt_count ?? 0,
            last_error: child.last_error,
        })),
    };
}

export function buildBatchResultExportFilename(batchID: string, exportedAt: Date = new Date()): string {
    const safeBatchID = batchID.replace(/[^A-Za-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || 'batch';
    const timestamp = exportedAt.toISOString().replace(/[:.]/g, '-');
    return `vm-batch-${safeBatchID}-${timestamp}.json`;
}

export function downloadBatchResultExport(batch: VMBatchStatus, exportedAt: Date = new Date()): void {
    const payload = buildBatchResultExportPayload(batch, exportedAt);
    const blob = new Blob([JSON.stringify(payload, null, 2)], {
        type: 'application/json;charset=utf-8',
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = buildBatchResultExportFilename(batch.batch_id, exportedAt);
    link.rel = 'noopener';
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
}
