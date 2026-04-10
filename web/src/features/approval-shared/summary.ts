import type { DescriptionsProps } from 'antd';
import type { TFunction } from 'i18next';

import type { components } from '@/types/api.gen';

type TicketSummaryData = components['schemas']['TicketSummary'];
type TicketItemSummaryData = components['schemas']['TicketItemSummary'];
type TicketRecord = {
    id: string;
    operation_type?: string;
    status: string;
    requester: string;
    requester_display_name?: string;
    requester_username?: string;
    approver?: string;
    approver_display_name?: string;
    approver_username?: string;
    reason?: string;
    ticket_payload?: Record<string, unknown> | null;
    request_prefill?: {
        system_id?: string;
        service_id?: string;
        template_id?: string;
        instance_size_id?: string;
        namespace?: string;
        reason?: string;
        batch_count?: number;
    };
    summary?: TicketSummaryData;
    target_vm_name?: string;
    created_at?: string;
    updated_at?: string;
};

const EMPTY_VALUE = '—';

type PayloadRecord = Record<string, unknown>;
type DescriptionItem = NonNullable<DescriptionsProps['items']>[number];

type ApprovalBatchDisplayItem = {
    key: string;
    title: string;
    scope?: string;
    cluster?: string;
    requestStatus?: string;
    latestStatus?: string;
    statusChanged?: boolean;
    currentShape?: string;
    targetShape?: string;
    action?: string;
};

type ApprovalActorSummary = {
    primary: string;
    secondary?: string;
};

type ApprovalSummarySections = {
    primary: string[];
    secondary: string[];
};

function buildApprovalBatchDisplayKey(
    entry: TicketItemSummaryData,
    index: number,
): string {
    const parts = [
        firstNonEmptyString(entry.vm_id, entry.template_id),
        firstNonEmptyString(entry.system_id),
        firstNonEmptyString(entry.service_id),
        firstNonEmptyString(entry.namespace),
        firstNonEmptyString(entry.cluster_id),
        firstNonEmptyString(entry.power_action),
        String(index),
    ].filter(Boolean);

    return parts.join(':') || `summary:${index}`;
}

function firstNonEmptyString(...values: Array<string | undefined | null>): string | undefined {
    for (const value of values) {
        if (typeof value === 'string' && value.trim() !== '') {
            return value.trim();
        }
    }
    return undefined;
}

function asPayloadRecord(value: unknown): PayloadRecord | undefined {
    return typeof value === 'object' && value !== null ? value as PayloadRecord : undefined;
}

function asPayloadRecords(value: unknown): PayloadRecord[] {
    if (!Array.isArray(value)) {
        return [];
    }
    return value
        .map(asPayloadRecord)
        .filter((item): item is PayloadRecord => Boolean(item));
}

function payloadString(value: unknown): string | undefined {
    return typeof value === 'string' && value.trim() !== '' ? value.trim() : undefined;
}

function payloadNumber(value: unknown): number | undefined {
    return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function payloadBool(value: unknown): boolean | undefined {
    return typeof value === 'boolean' ? value : undefined;
}

function requestPrefillString(
    requestPrefill: TicketRecord['request_prefill'],
    key: keyof NonNullable<TicketRecord['request_prefill']>,
): string | undefined {
    return payloadString(requestPrefill?.[key]);
}

function requestPrefillNumber(
    requestPrefill: TicketRecord['request_prefill'],
    key: keyof NonNullable<TicketRecord['request_prefill']>,
): number | undefined {
    return payloadNumber(requestPrefill?.[key]);
}

function compactDescriptionItems(items: Array<DescriptionItem | null>): DescriptionItem[] {
    return items.filter((item): item is DescriptionItem => Boolean(item));
}

function item(key: string, label: string, value: string | number | undefined): DescriptionItem | null {
    if (value === undefined || value === null || value === '') {
        return null;
    }
    return {
        key,
        label,
        children: value,
    };
}

function summarySystem(summary: TicketSummaryData | undefined): string | undefined {
    return firstNonEmptyString(summary?.system_name, summary?.system_id);
}

function summaryService(summary: TicketSummaryData | undefined): string | undefined {
    return firstNonEmptyString(summary?.service_name, summary?.service_id);
}

function summaryCluster(summary: TicketSummaryData | undefined): string | undefined {
    return firstNonEmptyString(summary?.cluster_name, summary?.cluster_id);
}

function summaryTemplate(summary: TicketSummaryData | undefined): string | undefined {
    return firstNonEmptyString(summary?.template_name, summary?.template_id);
}

function summaryInstanceSize(summary: TicketSummaryData | undefined): string | undefined {
    return firstNonEmptyString(summary?.instance_size_name, summary?.instance_size_id);
}

function summaryVM(summary: TicketSummaryData | undefined): string | undefined {
    return firstNonEmptyString(summary?.vm_name, summary?.vm_id);
}

function summaryRequestVMStatus(summary: TicketSummaryData | undefined): string | undefined {
    return firstNonEmptyString(summary?.request_vm_status);
}

function summaryLatestVMStatus(summary: TicketSummaryData | undefined): string | undefined {
    return firstNonEmptyString(summary?.latest_vm_status, summary?.vm_status);
}

function payloadTemplate(payload: PayloadRecord | undefined): string | undefined {
    return firstNonEmptyString(
        payloadString(payload?.template_label),
        payloadString(payload?.template_display_name),
        payloadString(payload?.template_name),
        payloadString(payload?.template_id),
    );
}

function payloadInstanceSize(payload: PayloadRecord | undefined): string | undefined {
    return firstNonEmptyString(
        payloadString(payload?.instance_size_label),
        payloadString(payload?.instance_size_display_name),
        payloadString(payload?.instance_size_name),
        payloadString(payload?.instance_size_id),
    );
}

function formatVMStatus(status: string | undefined, t: TFunction): string | undefined {
    if (!status) {
        return undefined;
    }
    const key = `vm:status.${status}`;
    const translated = t(key);
    return translated === key ? status : translated;
}

function formatPowerAction(action: string | undefined, t: TFunction): string | undefined {
    if (!action) {
        return undefined;
    }
    const key = `vm:batch.operation.${action}`;
    const translated = t(key);
    return translated === key ? action : translated;
}

export function formatApprovalResourceShape(
    cpu: number | undefined,
    memory: number | undefined,
    disk: number | undefined,
    t: TFunction,
): string | undefined {
    const parts: string[] = [];
    if (typeof cpu === 'number' && cpu > 0) {
        parts.push(t('summary.shape_cpu', { ns: 'approval', count: cpu }));
    }
    if (typeof memory === 'number' && memory > 0) {
        parts.push(t('summary.shape_memory', { ns: 'approval', count: memory }));
    }
    if (typeof disk === 'number' && disk > 0) {
        parts.push(t('summary.shape_disk', { ns: 'approval', count: disk }));
    }
    return parts.length > 0 ? parts.join(' · ') : undefined;
}

function formatBatchItemTitle(
    primary: string | undefined,
    secondary: string | undefined,
): string | undefined {
    if (primary && secondary && secondary !== primary) {
        return `${primary} · ${secondary}`;
    }
    return primary || secondary;
}

function fallbackBatchDisplayItems(
    ticket: TicketRecord,
    t: TFunction,
): ApprovalBatchDisplayItem[] {
    const payload = asPayloadRecord(ticket.ticket_payload);
    const items = asPayloadRecords(payload?.items);
    if (items.length === 0) {
        return [];
    }
        return items.map((entry, index) => ({
            key: `payload-${index}`,
            title: formatBatchItemTitle(
                firstNonEmptyString(payloadString(entry.vm_name), payloadTemplate(entry)),
                payloadInstanceSize(entry),
            ) ?? t('summary.item_fallback', { ns: 'approval', index: index + 1 }),
        scope: [
            firstNonEmptyString(payloadString(entry.system_name), payloadString(entry.system_id)),
            firstNonEmptyString(payloadString(entry.service_name), payloadString(entry.service_id)),
            payloadString(entry.namespace),
        ].filter(Boolean).join(' / '),
        cluster: firstNonEmptyString(payloadString(entry.cluster_name), payloadString(entry.cluster_id)),
        requestStatus: formatVMStatus(
            firstNonEmptyString(payloadString(entry.request_vm_status), payloadString(entry.vm_status)),
            t,
        ),
        latestStatus: formatVMStatus(
            firstNonEmptyString(payloadString(entry.latest_vm_status), payloadString(entry.vm_status)),
            t,
        ),
        statusChanged:
            firstNonEmptyString(payloadString(entry.request_vm_status), payloadString(entry.vm_status)) !==
            undefined &&
            firstNonEmptyString(payloadString(entry.latest_vm_status), payloadString(entry.vm_status)) !== undefined &&
            firstNonEmptyString(payloadString(entry.request_vm_status), payloadString(entry.vm_status)) !==
                firstNonEmptyString(payloadString(entry.latest_vm_status), payloadString(entry.vm_status)),
        currentShape: formatApprovalResourceShape(
        
            payloadNumber(entry.current_cpu_cores),
            payloadNumber(entry.current_memory_gi),
            payloadNumber(entry.current_disk_gb),
            t,
        ),
        targetShape: formatApprovalResourceShape(
            payloadNumber(entry.target_cpu_cores) ?? payloadNumber(entry.cpu_cores),
            payloadNumber(entry.target_memory_gi) ?? payloadNumber(entry.memory_gi),
            payloadNumber(entry.target_disk_gb) ?? payloadNumber(entry.instance_size_disk_gb),
            t,
        ),
        action: formatPowerAction(payloadString(entry.operation), t),
    }));
}

export function formatApprovalRecordID(id: string): string {
    if (id.length <= 14) {
        return id;
    }
    return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

export function approvalSummaryTitle(ticket: TicketRecord, t: TFunction): string {
    const summary = ticket.summary ?? undefined;
    const payload = asPayloadRecord(ticket.ticket_payload);

    switch (ticket.operation_type) {
        case 'DELETE':
        case 'MODIFY':
        case 'POWER':
        case 'VNC_ACCESS':
            return firstNonEmptyString(
                summaryVM(summary),
                payloadString(payload?.target_vm_name),
                payloadString(payload?.vm_name),
                ticket.reason,
                t(`op_type.${ticket.operation_type}`),
            ) ?? t(`op_type.${ticket.operation_type}`);
        case 'CREATE':
            return firstNonEmptyString(
                summaryTemplate(summary),
                payloadTemplate(payload),
                requestPrefillString(ticket.request_prefill, 'template_id'),
                t('op_type.CREATE'),
            ) ?? t('op_type.CREATE');
        default:
            return ticket.reason || t(`op_type.${ticket.operation_type ?? 'CREATE'}`);
    }
}

function approvalSummarySections(ticket: TicketRecord, t: TFunction): ApprovalSummarySections {
    const summary = ticket.summary ?? undefined;
    const payload = asPayloadRecord(ticket.ticket_payload);
    const primary: string[] = [];
    const secondary: string[] = [];
    const scope = [summarySystem(summary), summaryService(summary)].filter(Boolean).join(' / ');
    const namespace = firstNonEmptyString(
        summary?.namespace,
        requestPrefillString(ticket.request_prefill, 'namespace'),
        payloadString(payload?.namespace),
    );
    const cluster = summaryCluster(summary);
    const size = firstNonEmptyString(
        summaryInstanceSize(summary),
        payloadInstanceSize(payload),
        requestPrefillString(ticket.request_prefill, 'instance_size_id'),
    );
    const batchCount = summary?.batch_count ||
        requestPrefillNumber(ticket.request_prefill, 'batch_count') ||
        payloadNumber(payload?.batch_item_count);

    if (scope) {
        primary.push(scope);
    }
    if (namespace) {
        primary.push(namespace);
    }

    switch (ticket.operation_type) {
        case 'CREATE':
            if (size) {
                secondary.push(size);
            }
            if (cluster) {
                secondary.push(cluster);
            }
            break;
        case 'MODIFY': {
            const currentShape = formatApprovalResourceShape(
                summary?.current_cpu_cores,
                summary?.current_memory_gi,
                summary?.current_disk_gb,
                t,
            );
            const targetShape = formatApprovalResourceShape(
                summary?.target_cpu_cores,
                summary?.target_memory_gi,
                summary?.target_disk_gb,
                t,
            );
            const change = currentShape && targetShape && currentShape !== targetShape
                ? `${currentShape} → ${targetShape}`
                : targetShape || currentShape;
            if (change) {
                secondary.push(change);
            }
            if (cluster) {
                secondary.push(cluster);
            }
            if (payloadBool(payload?.requires_restart)) {
                secondary.push(t('summary.restart_required_short', { ns: 'approval' }));
            }
            break;
        }
        case 'DELETE':
        case 'VNC_ACCESS':
            if (cluster) {
                secondary.push(cluster);
            }
            const latestStatus = summaryLatestVMStatus(summary);
            if (latestStatus) {
                secondary.push(formatVMStatus(latestStatus, t) ?? latestStatus);
            }
            break;
        case 'POWER':
            if (cluster) {
                secondary.push(cluster);
            }
            if (summary?.power_action) {
                secondary.push(formatPowerAction(summary.power_action, t) ?? summary.power_action);
            }
            break;
        default:
            break;
    }

    if (batchCount && batchCount > 1) {
        secondary.push(t('request_batch_count', { ns: 'approval', count: batchCount }));
    }
    return { primary, secondary };
}

export function approvalSummaryMeta(ticket: TicketRecord, t: TFunction): string[] {
    const sections = approvalSummarySections(ticket, t);
    return [...sections.primary, ...sections.secondary];
}

function buildApprovalActorSummary(
    id: string | undefined,
    displayName: string | undefined,
    username: string | undefined,
): ApprovalActorSummary | undefined {
    const primary = firstNonEmptyString(displayName, username, id);
    if (!primary) {
        return undefined;
    }
    const normalizedDisplayName = firstNonEmptyString(displayName);
    const normalizedUsername = firstNonEmptyString(username);
    const secondary = normalizedDisplayName && normalizedUsername && normalizedDisplayName !== normalizedUsername
        ? normalizedUsername
        : undefined;
    return { primary, secondary };
}

export function approvalRequesterSummary(ticket: TicketRecord): ApprovalActorSummary | undefined {
    return buildApprovalActorSummary(
        ticket.requester,
        ticket.requester_display_name,
        ticket.requester_username,
    );
}

function approvalApproverSummary(ticket: TicketRecord): ApprovalActorSummary | undefined {
    return buildApprovalActorSummary(
        ticket.approver,
        ticket.approver_display_name,
        ticket.approver_username,
    );
}

export function buildApprovalOverviewItems(ticket: TicketRecord, t: TFunction): DescriptionItem[] {
    const summary = ticket.summary ?? undefined;
    const requester = approvalRequesterSummary(ticket);
    const approver = approvalApproverSummary(ticket);
    return compactDescriptionItems([
        item('requester', t('requester'), requester?.primary),
        item('approver', t('approver'), approver?.primary),
        item('reason', t('reason'), ticket.reason || undefined),
        item('batch_count', t('summary.batch_count', { ns: 'approval' }), summary?.batch_count && summary.batch_count > 1 ? summary.batch_count : undefined),
    ]);
}

export function buildApprovalScopeItems(ticket: TicketRecord, t: TFunction): DescriptionItem[] {
    const summary = ticket.summary ?? undefined;
    return compactDescriptionItems([
        item('system', t('summary.system', { ns: 'approval' }), summarySystem(summary)),
        item('service', t('summary.service', { ns: 'approval' }), summaryService(summary)),
        item('namespace', t('summary.namespace', { ns: 'approval' }), summary?.namespace || undefined),
        item('cluster', t('summary.cluster', { ns: 'approval' }), summaryCluster(summary)),
        item('cluster_environment', t('summary.cluster_environment', { ns: 'approval' }), summary?.cluster_environment || undefined),
        item('vm', t('summary.virtual_machine', { ns: 'approval' }), summaryVM(summary)),
        item('request_vm_status', t('summary.request_vm_status', { ns: 'approval' }), formatVMStatus(summaryRequestVMStatus(summary), t)),
        item('latest_vm_status', t('summary.latest_vm_status', { ns: 'approval' }), formatVMStatus(summaryLatestVMStatus(summary), t)),
    ]);
}

export function buildApprovalChangeItems(ticket: TicketRecord, t: TFunction): DescriptionItem[] {
    const summary = ticket.summary ?? undefined;
    const payload = asPayloadRecord(ticket.ticket_payload);
    const currentShape = formatApprovalResourceShape(
        summary?.current_cpu_cores,
        summary?.current_memory_gi,
        summary?.current_disk_gb,
        t,
    );
    const targetShape = formatApprovalResourceShape(
        summary?.target_cpu_cores,
        summary?.target_memory_gi,
        summary?.target_disk_gb,
        t,
    );
    const powerAction = formatPowerAction(summary?.power_action, t);
    const currentRequestShape = formatApprovalResourceShape(
        payloadNumber(payload?.current_cpu_request),
        payloadNumber(payload?.current_memory_request_gi),
        undefined,
        t,
    );
    return compactDescriptionItems([
        item('current_shape', t('summary.current_resources', { ns: 'approval' }), currentShape),
        item('target_shape', t('summary.target_resources', { ns: 'approval' }), targetShape),
        item('current_request_shape', t('summary.current_requests', { ns: 'approval' }), currentRequestShape),
        item(
            'restart_required',
            t('summary.apply_mode', { ns: 'approval' }),
            payloadBool(payload?.requires_restart)
                ? t('summary.restart_required_long', { ns: 'approval' })
                : undefined,
        ),
        item('power_action', t('summary.power_action', { ns: 'approval' }), powerAction),
    ]);
}

export function buildApprovalBatchDisplayItems(
    ticket: TicketRecord,
    t: TFunction,
): ApprovalBatchDisplayItem[] {
    const summaryItems = ticket.summary?.items ?? [];
    if (summaryItems.length > 0) {
        return summaryItems.map((entry, index) => ({
            key: buildApprovalBatchDisplayKey(entry, index),
            title: formatBatchItemTitle(
                firstNonEmptyString(entry.vm_name, entry.template_name, entry.vm_id, entry.template_id),
                firstNonEmptyString(entry.instance_size_name, entry.instance_size_id),
            ) ?? t('summary.item_fallback', { ns: 'approval', index: index + 1 }),
            scope: [firstNonEmptyString(entry.system_name, entry.system_id), firstNonEmptyString(entry.service_name, entry.service_id), entry.namespace].filter(Boolean).join(' / '),
            cluster: firstNonEmptyString(entry.cluster_name, entry.cluster_id),
            requestStatus: formatVMStatus(firstNonEmptyString(entry.request_vm_status), t),
            latestStatus: formatVMStatus(firstNonEmptyString(entry.latest_vm_status, entry.vm_status), t),
            statusChanged:
                firstNonEmptyString(entry.request_vm_status) !== undefined &&
                firstNonEmptyString(entry.latest_vm_status, entry.vm_status) !== undefined &&
                firstNonEmptyString(entry.request_vm_status) !== firstNonEmptyString(entry.latest_vm_status, entry.vm_status),
            currentShape: formatApprovalResourceShape(
                entry.current_cpu_cores,
                entry.current_memory_gi,
                entry.current_disk_gb,
                t,
            ),
            targetShape: formatApprovalResourceShape(
                entry.target_cpu_cores,
                entry.target_memory_gi,
                entry.target_disk_gb,
                t,
            ),
            action: formatPowerAction(entry.power_action, t),
        }));
    }
    return fallbackBatchDisplayItems(ticket, t);
}

export function approvalPrimaryAlert(
    ticket: TicketRecord,
    t: TFunction,
): { type: 'warning' | 'info'; message: string; description?: string } | null {
    if (ticket.summary?.irreversible) {
        return {
            type: 'warning',
            message: t('summary.irreversible_title', { ns: 'approval' }),
            description: t('summary.irreversible_description', { ns: 'approval' }),
        };
    }
    if (ticket.operation_type === 'MODIFY') {
        return {
            type: 'info',
            message: t('summary.modify_title', { ns: 'approval' }),
            description: t('summary.modify_description', { ns: 'approval' }),
        };
    }
    return null;
}

export function approvalEmptyValue(): string {
    return EMPTY_VALUE;
}
