import dayjs from 'dayjs';
import type { ElementType } from 'react';
import { DeleteOutlined, PlusCircleOutlined } from '@ant-design/icons';

import type { components } from '@/types/api.gen';

export type ApprovalTicket = components['schemas']['ApprovalTicket'];
export type ApprovalTicketList = components['schemas']['ApprovalTicketList'];
export type ApprovalDecisionRequest = components['schemas']['ApprovalDecisionRequest'];
export type RejectDecisionRequest = components['schemas']['RejectDecisionRequest'];
export type ClusterList = components['schemas']['ClusterList'];
export type Cluster = components['schemas']['Cluster'];
export type ApprovalStatus = NonNullable<ApprovalTicket['status']>;

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

export const OP_TYPE_CONFIG: Record<string, { color: string; icon: ElementType }> = {
    CREATE: { color: 'blue', icon: PlusCircleOutlined },
    DELETE: { color: 'red', icon: DeleteOutlined },
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
    { key: 'APPROVED', i18nKey: 'filter.approved' },
    { key: 'REJECTED', i18nKey: 'filter.rejected' },
    { key: 'CANCELLED', i18nKey: 'filter.cancelled' },
    { key: 'ALL', i18nKey: 'filter.all' },
];
