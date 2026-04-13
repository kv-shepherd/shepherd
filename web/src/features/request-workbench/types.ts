import type { components } from '@/types/api.gen';

export type ApprovalStatus = 'PENDING' | 'APPROVED' | 'REJECTED' | 'CANCELLED' | 'EXECUTING' | 'SUCCESS' | 'FAILED';

export interface VMRequestPrefill {
    system_id: string;
    service_id: string;
    template_id: string;
    instance_size_id: string;
    namespace: string;
    reason: string;
    batch_count: number;
}

export interface Ticket {
    id: string;
    event_id?: string;
    operation_type?: string;
    status: ApprovalStatus;
    requester: string;
    requester_display_name?: string;
    requester_username?: string;
    reason?: string;
    approver?: string;
    approver_display_name?: string;
    approver_username?: string;
    reject_reason?: string;
    created_at: string;
    updated_at?: string;
    request_prefill?: VMRequestPrefill;
    summary?: components['schemas']['TicketSummary'];
    target_vm_name?: string;
    ticket_payload?: Record<string, unknown>;
    provisioning?: components['schemas']['ProvisioningStatus'];
}

export interface TicketList {
    items: Ticket[];
    pagination?: { total: number; page: number; per_page: number };
}

export type RequestTicketOperationType = 'CREATE' | 'MODIFY' | 'DELETE' | 'POWER' | 'VNC_ACCESS';
export type RequestWorkbenchView = 'drafts' | 'in_progress' | 'history';
export type HistoryStatusFilter = 'ALL' | Extract<ApprovalStatus, 'SUCCESS' | 'FAILED' | 'REJECTED' | 'CANCELLED'>;

export const STATUS_COLORS: Record<ApprovalStatus, string> = {
    PENDING: 'orange',
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
