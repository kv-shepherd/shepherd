import type { ApiErrorResponse } from '@/hooks/useApiQuery';
import { hasPermission } from '@/lib/auth/permissions';
import type { components } from '@/types/api.gen';

type UserInfo = components['schemas']['UserInfo'];
type VMBatchActionResponse = components['schemas']['VMBatchActionResponse'];
type VMBatchChildStatus = components['schemas']['VMBatchChildStatus'];

export type BatchActionKind = 'retry' | 'cancel';

export interface BatchActionFeedback {
    action: BatchActionKind;
    affectedCount: number;
    affectedTicketIDs: string[];
}

export interface RestartReconciliationNotice {
    eventId: string;
    reconciliationPath: string;
}

const OPERATION_PERMISSIONS: Record<string, string> = {
    CREATE: 'vm:create',
    DELETE: 'vm:delete',
    MODIFY: 'vm:operate',
    POWER: 'vm:operate',
    START: 'vm:operate',
    STOP: 'vm:operate',
    RESTART: 'vm:operate',
};

const AMBIGUOUS_RESTART_RUNBOOK = 'operator-runbook:ambiguous-vm-restart';

const canonicalizeBatchIntentValue = (value: unknown): unknown => {
    if (Array.isArray(value)) {
        return value
            .map(canonicalizeBatchIntentValue)
            .sort((left, right) => {
                const leftJSON = JSON.stringify(left) ?? '';
                const rightJSON = JSON.stringify(right) ?? '';
                return leftJSON < rightJSON ? -1 : leftJSON > rightJSON ? 1 : 0;
            });
    }
    if (value !== null && typeof value === 'object') {
        return Object.fromEntries(
            Object.entries(value)
                .sort(([left], [right]) => left.localeCompare(right))
                .map(([key, nested]) => [key, canonicalizeBatchIntentValue(nested)]),
        );
    }
    return value;
};

export const createCanonicalBatchIntentFingerprint = (payload: unknown): string =>
    JSON.stringify(canonicalizeBatchIntentValue(payload)) ?? 'null';

export const createOpaqueBatchRequestId = (): string => {
    if (typeof globalThis.crypto?.randomUUID !== 'function') {
        throw new Error('secure UUID generation is unavailable');
    }
    return globalThis.crypto.randomUUID();
};

export const isRetryableBatchChild = (child: VMBatchChildStatus): boolean =>
    child.status === 'FAILED' && (child.attempt_count ?? 0) < 3;

export const canManageBatchOperation = (
    user: UserInfo | null | undefined,
    operation: string | undefined,
): boolean => {
    if (!user || !operation) {
        return false;
    }
    const permission = OPERATION_PERMISSIONS[operation.trim().toUpperCase()];
    if (!permission) {
        return false;
    }
    if (hasPermission(user, 'builtin_approval:approve')) {
        return true;
    }
    return hasPermission(user, permission);
};

export const createBatchActionFeedback = (
    action: BatchActionKind,
    response: VMBatchActionResponse,
    fallbackTicketIDs: readonly string[] = [],
): BatchActionFeedback => {
    const rawAffectedCount = Number(response.affected_count ?? 0);
    const affectedCount = Number.isFinite(rawAffectedCount)
        ? Math.max(0, Math.floor(rawAffectedCount))
        : 0;
    const responseIDs = Array.from(new Set(
        (response.affected_ticket_ids ?? []).map((id) => id.trim()).filter(Boolean),
    ));
    const affectedTicketIDs = responseIDs.length > 0
        ? responseIDs
        : fallbackTicketIDs
            .map((id) => id.trim())
            .filter(Boolean)
            .slice(0, affectedCount);

    return { action, affectedCount, affectedTicketIDs };
};

export const extractBatchRetryAfterSeconds = (error: ApiErrorResponse): number => {
    const candidate = error.retry_after_seconds ?? error.params?.retry_after_seconds;
    const parsed = Number(candidate);
    if (!Number.isFinite(parsed) || parsed <= 0) {
        return 0;
    }
    return Math.ceil(parsed);
};

export const isBatchConflictError = (error: ApiErrorResponse): boolean =>
    error.status === 409 || error.code.endsWith('_CONFLICT');

export const extractRestartReconciliationNotice = (
    error: ApiErrorResponse,
): RestartReconciliationNotice | null => {
    if (
        (error.code !== 'POWER_OPERATION_IN_PROGRESS' && error.code !== 'DUPLICATE_PENDING_REQUEST')
        || error.params?.operator_action_required !== true
    ) {
        return null;
    }
    const eventId = typeof error.params.existing_event_id === 'string'
        ? error.params.existing_event_id.trim()
        : '';
    const reconciliationPath = typeof error.params.reconciliation_path === 'string'
        ? error.params.reconciliation_path.trim()
        : '';
    if (!eventId || reconciliationPath !== AMBIGUOUS_RESTART_RUNBOOK) {
        return null;
    }
    return { eventId, reconciliationPath };
};
