import dayjs from 'dayjs';

import type { components } from '@/types/api.gen';

export type ApprovalTask = components['schemas']['Ticket'];
export type ApprovalTaskList = components['schemas']['TicketList'];
export type ApprovalDecisionRequest = components['schemas']['ApprovalDecisionRequest'];
export type RejectDecisionRequest = components['schemas']['RejectDecisionRequest'];
export type ClusterList = components['schemas']['ClusterList'];
export type Cluster = components['schemas']['Cluster'];
export type ApprovalStatus = NonNullable<ApprovalTask['status']>;

export const STATUS_COLORS: Record<string, string> = {
    PENDING: 'gold',
    APPROVED: 'green',
    REJECTED: 'red',
    CANCELLED: 'default',
    EXECUTING: 'blue',
    SUCCESS: 'green',
    FAILED: 'red',
};

export const STATUS_BADGES: Record<string, 'processing' | 'success' | 'error' | 'default'> = {
    PENDING: 'processing',
    APPROVED: 'success',
    REJECTED: 'error',
    CANCELLED: 'default',
    EXECUTING: 'processing',
    SUCCESS: 'success',
    FAILED: 'error',
};

/** ADR-0015 §11: visual priority by pending duration. */
export const getPriorityTier = (createdAt?: string): 'urgent' | 'warning' | 'normal' => {
    if (!createdAt) {
        return 'normal';
    }
    const days = dayjs().diff(dayjs(createdAt), 'day');
    if (days > 7) {
        return 'urgent';
    }
    if (days > 3) {
        return 'warning';
    }
    return 'normal';
};

export const STATUS_FILTER_OPTIONS: Array<{ key: ApprovalStatus | 'ALL'; i18nKey: string }> = [
    { key: 'PENDING', i18nKey: 'filter.pending' },
    { key: 'EXECUTING', i18nKey: 'filter.executing' },
    { key: 'SUCCESS', i18nKey: 'filter.success' },
    { key: 'FAILED', i18nKey: 'filter.failed' },
    { key: 'REJECTED', i18nKey: 'filter.rejected' },
    { key: 'CANCELLED', i18nKey: 'filter.cancelled' },
    { key: 'ALL', i18nKey: 'filter.all' },
];

export const OPERATION_FILTER_OPTIONS: Array<{ key: ApprovalTask['operation_type'] | 'ALL'; i18nKey: string }> = [
    { key: 'ALL', i18nKey: 'filter.operation_all' },
    { key: 'CREATE', i18nKey: 'op_type.CREATE' },
    { key: 'MODIFY', i18nKey: 'op_type.MODIFY' },
    { key: 'DELETE', i18nKey: 'op_type.DELETE' },
    { key: 'POWER', i18nKey: 'op_type.POWER' },
    { key: 'VNC_ACCESS', i18nKey: 'op_type.VNC_ACCESS' },
];
