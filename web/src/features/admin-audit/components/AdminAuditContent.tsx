'use client';

import { useMemo, useState } from 'react';
import {
    AutoComplete,
    Button,
    Collapse,
    Descriptions,
    Drawer,
    Flex,
    Select,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { DescriptionsProps } from 'antd/es/descriptions';
import { EyeOutlined, ReloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    NotificationInboxGlyph,
    QueueReviewGlyph,
    RequestsOverviewGlyph,
    ServiceWorkspaceGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

import { buildAuditLogQuery, type AuditLogFilters } from '../query';

const { Text, Title } = Typography;

type AuditLog = components['schemas']['AuditLog'];
type AuditLogList = components['schemas']['AuditLogList'];
type AuditPlacementSummary = components['schemas']['AuditPlacementSummary'];
type TicketSummary = components['schemas']['TicketSummary'];
type TicketItemSummary = components['schemas']['TicketItemSummary'];
type AuditTranslation = (key: string, options?: Record<string, unknown>) => string;
type AuditFeedBadge = { key: string; label: string; color?: string };
type AuditPresetKey = '' | 'requests' | 'approvals' | 'resource_changes' | 'system_tasks';

const ACTION_COLORS: Record<string, string> = {
    CREATE: 'green',
    UPDATE: 'blue',
    DELETE: 'red',
    APPROVE: 'cyan',
    REJECT: 'orange',
    START: 'green',
    STOP: 'gold',
    RESTART: 'purple',
    LOGIN: 'geekblue',
    ACCESS: 'geekblue',
    REQUESTED: 'blue',
    SUBMIT: 'gold',
};

const DECISION_COLORS: Record<string, string> = {
    approved: 'green',
    rejected: 'red',
    validation_failed: 'orange',
    power_approved: 'cyan',
    delete_approved: 'volcano',
    vnc_access_approved: 'blue',
    batch_approved: 'green',
    batch_rejected: 'red',
    cancelled: 'default',
    batch_cancelled: 'default',
};

const COMMON_AUDIT_ACTIONS = [
    'user.login',
    'user.password_change',
    'vm.request',
    'vm.delete_requested',
    'vm.modify_requested',
    'vm.start_requested',
    'vm.stop_requested',
    'vm.restart_requested',
    'vm.batch.submit',
    'vm.batch.power.submit',
    'vnc.request_submitted',
    'vnc.access',
    'approval.approved',
    'approval.rejected',
    'approval.validation_failed',
    'approval.power_approved',
    'approval.delete_approved',
    'approval.vnc_access_approved',
    'approval.cancelled',
    'approval.batch_approved',
    'approval.batch_rejected',
    'approval.batch_cancelled',
    'auth_provider.directory_sync_requested',
    'auth_provider.directory_sync',
    'auth_provider.directory_sync_failed',
    'auth_provider.directory_enrichment_scheduled',
] as const;

function normalizeActionKey(action?: string): string {
    return (action ?? '').trim().toLowerCase().replace(/[.\s-]+/g, '_');
}

function actionSuffix(action?: string): string {
    const normalized = normalizeActionKey(action);
    const tokens = normalized.split('_').filter(Boolean);
    return tokens.at(-1) ?? normalized;
}

function prettifyAuditToken(value?: string): string {
    if (!value) {
        return '—';
    }
    return value
        .trim()
        .replace(/[._-]+/g, ' ')
        .replace(/\s+/g, ' ')
        .split(' ')
        .filter(Boolean)
        .map((token) => token.charAt(0).toUpperCase() + token.slice(1))
        .join(' ');
}

function translateAuditAction(t: AuditTranslation, action?: string): string {
    const normalized = normalizeActionKey(action);
    const normalizedLabel = t(`audit.action_code.${normalized}`, { defaultValue: '' });
    if (normalizedLabel) {
        return normalizedLabel;
    }
    const suffix = actionSuffix(action);
    const suffixLabel = t(`audit.action_code.${suffix}`, { defaultValue: '' });
    if (suffixLabel) {
        return suffixLabel;
    }
    return prettifyAuditToken(action);
}

function translateAuditResourceType(t: AuditTranslation, resourceType?: string): string {
    const normalized = (resourceType ?? '').trim().toLowerCase();
    return t(`audit.resource_option.${normalized}`, {
        defaultValue: prettifyAuditToken(normalized),
    });
}

function translateAuditDecision(t: AuditTranslation, decision?: string): string {
    if (!decision) {
        return '';
    }
    return t(`audit.decision_option.${decision}`, { defaultValue: prettifyAuditToken(decision) });
}

function systemActorCode(actor?: string): string {
    const trimmed = actor?.trim() ?? '';
    if (!trimmed.startsWith('system:')) {
        return '';
    }
    return trimmed.slice('system:'.length).trim();
}

function translateAuditSystemActor(
    t: AuditTranslation,
    actor?: string,
): { displayName: string; secondary: string } | null {
    const code = systemActorCode(actor);
    if (!code) {
        return null;
    }
    const normalized = normalizeActionKey(code);
    return {
        displayName: t('audit.actor_system', { defaultValue: 'System task' }),
        secondary: t(`audit.system_actor.${normalized}`, {
            defaultValue: prettifyAuditToken(code),
        }),
    };
}

function translateAuditDirectorySyncMode(t: AuditTranslation, value?: string): string {
    const normalized = normalizeActionKey(value);
    return t(`audit.sync_mode.${normalized}`, {
        defaultValue: prettifyAuditToken(value),
    });
}

function translateAuditDirectorySyncStatus(t: AuditTranslation, value?: string): string {
    const normalized = normalizeActionKey(value);
    return t(`audit.sync_status.${normalized}`, {
        defaultValue: prettifyAuditToken(value),
    });
}

function normalizeAuditFilters(filters: AuditLogFilters): AuditLogFilters {
    return {
        search: filters.search.trim(),
        category: filters.category,
        action: filters.action.trim(),
        approval_decision: filters.approval_decision.trim(),
        actor: filters.actor.trim(),
        placement_advisory_code: filters.placement_advisory_code.trim(),
        placement_reason_code: filters.placement_reason_code.trim(),
        resource_type: filters.resource_type.trim(),
        resource_id: filters.resource_id.trim(),
    };
}

function dedupeSortedStrings(values: Array<string | undefined>): string[] {
    return Array.from(
        new Set(
            values
                .map((value) => value?.trim())
                .filter((value): value is string => Boolean(value)),
        ),
    ).sort((left, right) => left.localeCompare(right));
}

function joinAuditParts(...parts: Array<string | undefined>): string {
    return listAuditParts(...parts).join(' · ');
}

function listAuditParts(...parts: Array<string | undefined>): string[] {
    return Array.from(
        new Set(
            parts
                .map((part) => part?.trim())
                .filter((part): part is string => Boolean(part)),
        ),
    );
}

function shortAuditID(value?: string): string {
    const trimmed = value?.trim() ?? '';
    if (trimmed.length <= 18) {
        return trimmed || '—';
    }
    return `${trimmed.slice(0, 12)}…${trimmed.slice(-4)}`;
}

function actorPrimary(record: AuditLog, t: AuditTranslation): string {
    const summaryDisplay = record.actor_summary?.display_name?.trim();
    if (summaryDisplay) {
        return summaryDisplay;
    }
    const systemActor = translateAuditSystemActor(t, record.actor);
    if (systemActor) {
        return systemActor.displayName;
    }
    return record.actor?.trim() || '—';
}

function actorSecondary(record: AuditLog, t: AuditTranslation): string {
    const summarySecondary = record.actor_summary?.secondary?.trim();
    if (summarySecondary) {
        return summarySecondary;
    }
    const systemActor = translateAuditSystemActor(t, record.actor);
    return systemActor?.secondary ?? '';
}

function isUserIdentityAuditRecord(record: AuditLog): boolean {
    const normalizedAction = normalizeActionKey(record.action);
    return record.resource_type === 'user' && (
        normalizedAction === 'user_login' || normalizedAction === 'user_password_change'
    );
}

function isLowSignalAuditRecord(record: AuditLog): boolean {
    if (record.resource_type === 'directory_sync_job') {
        return true;
    }
    return Boolean(systemActorCode(record.actor)) && record.resource_type !== 'ticket';
}

function resourcePrimary(record: AuditLog): string {
    return record.resource_summary?.display_name?.trim() || record.resource_id?.trim() || '—';
}

function auditDetailString(details: AuditLog['details'], key: string): string {
    if (!details || typeof details !== 'object') {
        return '';
    }
    return formatAuditValue(details[key]);
}

function auditDetailNumber(details: AuditLog['details'], key: string): number | undefined {
    if (!details || typeof details !== 'object') {
        return undefined;
    }
    const value = details[key];
    if (typeof value === 'number' && Number.isFinite(value)) {
        return value;
    }
    if (typeof value === 'string') {
        const parsed = Number(value);
        if (Number.isFinite(parsed)) {
            return parsed;
        }
    }
    return undefined;
}

function buildDirectorySyncResult(record: AuditLog, t: AuditTranslation): string {
    const parts: string[] = [];
    const totalEntries = auditDetailNumber(record.details, 'total_entries');
    const createCount = auditDetailNumber(record.details, 'create_count');
    const updateCount = auditDetailNumber(record.details, 'update_count');
    const blockedCount = auditDetailNumber(record.details, 'blocked_count');
    const errorCount = auditDetailNumber(record.details, 'error_count');
    if (totalEntries !== undefined && totalEntries > 0) {
        parts.push(t('audit.sync_metric.total', { defaultValue: '{{count}} total', count: totalEntries }));
    }
    if (createCount !== undefined && createCount > 0) {
        parts.push(t('audit.sync_metric.created', { defaultValue: '{{count}} created', count: createCount }));
    }
    if (updateCount !== undefined && updateCount > 0) {
        parts.push(t('audit.sync_metric.updated', { defaultValue: '{{count}} updated', count: updateCount }));
    }
    if (blockedCount !== undefined && blockedCount > 0) {
        parts.push(t('audit.sync_metric.blocked', { defaultValue: '{{count}} blocked', count: blockedCount }));
    }
    if (errorCount !== undefined && errorCount > 0) {
        parts.push(t('audit.sync_metric.errors', { defaultValue: '{{count}} errors', count: errorCount }));
    }
    if (parts.length === 0) {
        const status = record.resource_summary?.tertiary?.trim();
        if (status) {
            parts.push(translateAuditDirectorySyncStatus(t, status));
        }
    }
    return joinAuditParts(...parts);
}

function buildAuditHeadline(record: AuditLog, t: AuditTranslation): string {
    if (record.resource_type === 'directory_sync_job') {
        return resourcePrimary(record);
    }
    if (record.resource_type === 'user') {
        const normalizedAction = normalizeActionKey(record.action);
        if (normalizedAction === 'user_login' || normalizedAction === 'user_password_change') {
            return resourcePrimary(record);
        }
    }
    const normalizedAction = normalizeActionKey(record.action);
    const actionHeadline = t(`audit.headline.${normalizedAction}`, { defaultValue: '' }).trim();
    if (actionHeadline) {
        return actionHeadline;
    }
    if (record.resource_type === 'ticket') {
        const hasReadableResourceName = Boolean(record.resource_summary?.display_name?.trim());
        const isBatchTicket = (record.ticket_summary?.batch_count ?? 0) > 1;
        if (isBatchTicket || !hasReadableResourceName) {
            return translateAuditAction(t, record.action);
        }
    }
    return resourcePrimary(record);
}

function buildAuditFeedMeta(record: AuditLog, t: AuditTranslation): string[] {
    const headline = buildAuditHeadline(record, t);
    const resourceName = resourcePrimary(record);
    const actorName = actorPrimary(record, t);

    if (isUserIdentityAuditRecord(record)) {
        return listAuditParts(
            translateAuditAction(t, record.action),
            resourceName !== headline ? resourceName : '',
            record.resource_summary?.secondary,
            actorSecondary(record, t),
        );
    }

    if (record.resource_type === 'ticket') {
        return listAuditParts(
            buildTicketScope(record.ticket_summary),
            buildTicketTarget(record.ticket_summary),
            ticketBatchLabel(record.ticket_summary, t),
            ticketRequesterLabel(record.ticket_summary),
            ticketApproverLabel(record.ticket_summary),
        );
    }

    if (record.resource_type === 'directory_sync_job') {
        return listAuditParts(
            record.resource_summary?.secondary
                ? translateAuditDirectorySyncMode(t, record.resource_summary.secondary)
                : '',
            record.resource_summary?.tertiary
                ? translateAuditDirectorySyncStatus(t, record.resource_summary.tertiary)
                : '',
            actorSecondary(record, t),
        );
    }

    return listAuditParts(
        resourceName !== headline ? resourceName : '',
        record.resource_summary?.secondary,
        record.resource_summary?.tertiary,
        actorName !== headline && actorName !== resourceName ? actorName : '',
        actorSecondary(record, t),
    );
}

function buildAuditFeedBadges(record: AuditLog, t: AuditTranslation): AuditFeedBadge[] {
    const actionLabel = translateAuditAction(t, record.action);
    const headline = buildAuditHeadline(record, t);
    const badges: AuditFeedBadge[] = [];
    if (actionLabel !== headline) {
        badges.push({
            key: 'action',
            label: actionLabel,
            color: ACTION_COLORS[actionSuffix(record.action).toUpperCase()] ?? 'default',
        });
    }
    if (record.approval_decision) {
        const decisionLabel = translateAuditDecision(t, record.approval_decision);
        if (decisionLabel === actionLabel || decisionLabel === headline) {
            return badges;
        }
        badges.push({
            key: 'decision',
            label: decisionLabel,
            color: DECISION_COLORS[record.approval_decision] ?? 'blue',
        });
    }
    if (
        record.resource_type !== 'ticket' &&
        record.resource_type !== 'directory_sync_job' &&
        !(record.resource_type === 'user' && ['user_login', 'user_password_change'].includes(normalizeActionKey(record.action)))
    ) {
        badges.push({
            key: 'resource-type',
            label: translateAuditResourceType(t, record.resource_type),
        });
    }
    return badges;
}

function ticketRequesterLabel(summary?: TicketSummary): string {
    return joinAuditParts(summary?.requester_display_name, summary?.requester_username);
}

function ticketApproverLabel(summary?: TicketSummary): string {
    return joinAuditParts(summary?.approver_display_name, summary?.approver_username);
}

function ticketOwnerLabel(summary?: TicketSummary): string {
    return joinAuditParts(summary?.owner_display_name, summary?.owner_username);
}

function ticketItemOwnerLabel(item?: TicketItemSummary): string {
    return joinAuditParts(item?.owner_display_name, item?.owner_username);
}

function ticketBatchLabel(summary: TicketSummary | undefined, t: AuditTranslation): string {
    const count = summary?.batch_count ?? 0;
    if (count <= 1) {
        return '';
    }
    return t('audit.context.batch_items', {
        defaultValue: '{{count}} items',
        count,
    });
}

function uniqueTicketItemValues(
    summary: TicketSummary | undefined,
    selector: (item: TicketItemSummary) => string,
): string[] {
    const values = new Set<string>();
    for (const item of summary?.items ?? []) {
        const value = selector(item).trim();
        if (value) {
            values.add(value);
        }
    }
    return Array.from(values);
}

function summarizeOverflow(values: string[], limit = 2): string {
    if (values.length === 0) {
        return '';
    }
    if (values.length <= limit) {
        return values.join(' · ');
    }
    return `${values.slice(0, limit).join(' · ')} +${values.length - limit}`;
}

function buildTicketScope(summary: TicketSummary | undefined): string {
    const sharedScope = joinAuditParts(summary?.system_name, summary?.service_name);
    if (sharedScope) {
        return sharedScope;
    }
    return joinAuditParts(
        summarizeOverflow(uniqueTicketItemValues(summary, (item) => item.system_name ?? ''), 1),
        summarizeOverflow(uniqueTicketItemValues(summary, (item) => item.service_name ?? ''), 2),
    );
}

function buildTicketTarget(summary: TicketSummary | undefined): string {
    if (!summary) {
        return '';
    }
    const vmNames = uniqueTicketItemValues(summary, (item) => item.vm_name ?? '');
    const namespaces = uniqueTicketItemValues(summary, (item) => item.namespace ?? '');
    const clusterNames = uniqueTicketItemValues(summary, (item) => item.cluster_name ?? '');
    const environments = uniqueTicketItemValues(summary, (item) => item.cluster_environment ?? '');
    if ((summary.batch_count ?? 0) > 1) {
        return joinAuditParts(
            summarizeOverflow(vmNames),
            summarizeOverflow(namespaces),
            summarizeOverflow(clusterNames, 1),
            summarizeOverflow(environments, 1),
            !namespaces.length ? summary.namespace : '',
            !clusterNames.length ? summary.cluster_name : '',
            !environments.length ? summary.cluster_environment : '',
        );
    }
    return joinAuditParts(
        summary.vm_name,
        summary.namespace || namespaces[0],
        summary.cluster_name || clusterNames[0],
        summary.cluster_environment || environments[0],
    );
}

function buildTicketRequestedChange(summary: TicketSummary | undefined): string {
    if (!summary) {
        return '';
    }
    const sharedChange = joinAuditParts(
        summary.template_name,
        summary.instance_size_name,
        summarizeTargetResources(summary),
        summary.power_action,
    );
    if ((summary.batch_count ?? 0) <= 1 || sharedChange) {
        return sharedChange;
    }
    const itemChanges = uniqueTicketItemValues(summary, (item) => joinAuditParts(
        item.template_name,
        item.instance_size_name,
        summarizeTargetResources(item),
        item.power_action,
    ));
    return summarizeOverflow(itemChanges);
}

function buildTicketBatchItemCards(summary: TicketSummary, t: AuditTranslation): Array<{
    key: string;
    title: string;
    subtitle: string;
    lines: Array<{ label: string; value: string }>;
}> {
    return (summary.items ?? []).map((item, index) => {
        const scope = joinAuditParts(item.system_name, item.service_name);
        const target = joinAuditParts(item.namespace, item.cluster_name, item.cluster_environment);
        const owner = ticketItemOwnerLabel(item);
        const requested = joinAuditParts(
            item.template_name,
            item.instance_size_name,
            summarizeTargetResources(item),
            item.power_action,
        );
        return {
            key: item.vm_id || item.vm_name || `${index}`,
            title: item.vm_name || t('audit.batch_item.pending_vm', {
                defaultValue: 'Pending VM #{{count}}',
                count: index + 1,
            }),
            subtitle: item.vm_name ? scope : joinAuditParts(scope, requested),
            lines: [
                { label: 'audit.context.scope', value: scope },
                { label: 'audit.context.owner', value: owner },
                { label: 'audit.context.target', value: target },
                { label: 'audit.context.requested_change', value: requested },
            ].filter((line) => line.value),
        };
    });
}

function buildContextRows(record: AuditLog, t: AuditTranslation): Array<{ key: string; label: string; value: string }> {
    const rows: Array<{ key: string; label: string; value: string }> = [];
    const summary = record.ticket_summary;

    if (record.resource_type === 'directory_sync_job') {
        const result = buildDirectorySyncResult(record, t);
        if (result) {
            rows.push({
                key: 'result',
                label: t('audit.context.result', { defaultValue: 'Result' }),
                value: result,
            });
        }
        const joinKey = auditDetailString(record.details, 'join_key_type') || auditDetailString(record.details, 'join_key');
        if (joinKey) {
            rows.push({
                key: 'join-key',
                label: t('audit.detail_field.join_key', { defaultValue: 'Join key' }),
                value: prettifyAuditToken(joinKey),
            });
        }
        const scheduler = actorSecondary(record, t);
        if (scheduler) {
            rows.push({
                key: 'triggered-by',
                label: t('audit.context.triggered_by', { defaultValue: 'Triggered by' }),
                value: scheduler,
            });
        }
        return rows;
    }

    if (isUserIdentityAuditRecord(record)) {
        return rows;
    }

    if (summary) {
        const requester = ticketRequesterLabel(summary);
        if (requester) {
            rows.push({
                key: 'requester',
                label: t('audit.context.requester', { defaultValue: 'Requester' }),
                value: requester,
            });
        }
        const approver = ticketApproverLabel(summary);
        if (approver) {
            rows.push({
                key: 'approver',
                label: t('audit.context.approver', { defaultValue: 'Approver' }),
                value: approver,
            });
        }
        const owner = ticketOwnerLabel(summary);
        if (owner) {
            rows.push({
                key: 'owner',
                label: t('audit.context.owner', { defaultValue: 'Owner' }),
                value: owner,
            });
        }
        const scope = buildTicketScope(summary);
        if (scope) {
            rows.push({
                key: 'scope',
                label: t('audit.context.scope', { defaultValue: 'Scope' }),
                value: scope,
            });
        }

        const target = buildTicketTarget(summary);
        if (target) {
            rows.push({
                key: 'target',
                label: t('audit.context.target', { defaultValue: 'Target' }),
                value: target,
            });
        }

        const requestedResources = buildTicketRequestedChange(summary);
        if (requestedResources) {
            rows.push({
                key: 'requested',
                label: t('audit.context.requested_change', { defaultValue: 'Requested change' }),
                value: requestedResources,
            });
        }
    } else {
        const scope = joinAuditParts(
            record.resource_summary?.secondary,
            record.resource_summary?.tertiary,
        );
        if (scope) {
            rows.push({
                key: 'context',
                label: t('audit.context.context', { defaultValue: 'Context' }),
                value: scope,
            });
        }
    }

    if (record.placement_summary) {
        const placement = joinAuditParts(
            record.placement_summary.selected_cluster_name || record.placement_summary.selected_cluster_id,
            record.placement_summary.reason_code,
            record.placement_summary.advisory_code,
        );
        if (placement) {
            rows.push({
                key: 'placement',
                label: t('audit.context.placement', { defaultValue: 'Placement' }),
                value: placement,
            });
        }
    }

    if (rows.length === 0) {
        const actorContext = actorSecondary(record, t);
        if (actorContext) {
            rows.push({
                key: 'actor',
                label: t('audit.context.actor_identity', { defaultValue: 'Actor identity' }),
                value: actorContext,
            });
        }
    }

    return rows;
}

function summarizeTargetResources(summary: TicketSummary): string {
    const parts: string[] = [];
    if ((summary.target_cpu_cores ?? 0) > 0) {
        parts.push(`${trimFloat(summary.target_cpu_cores)} vCPU`);
    }
    if ((summary.target_memory_gi ?? 0) > 0) {
        parts.push(`${trimFloat(summary.target_memory_gi)} Gi`);
    }
    if ((summary.target_disk_gb ?? 0) > 0) {
        parts.push(`${summary.target_disk_gb} Gi`);
    }
    return parts.join(' · ');
}

function trimFloat(value?: number): string {
    if (!value) {
        return '0';
    }
    if (Number.isInteger(value)) {
        return `${value}`;
    }
    return value.toFixed(2).replace(/\.?0+$/, '');
}

function translateAuditFieldLabel(t: AuditTranslation, key: string): string {
    return t(`audit.detail_field.${key}`, { defaultValue: prettifyAuditToken(key) });
}

function formatAuditValue(value: unknown): string {
    if (typeof value === 'string') {
        return value.trim();
    }
    if (typeof value === 'number' || typeof value === 'boolean') {
        return String(value);
    }
    if (Array.isArray(value)) {
        return value
            .map((item) => formatAuditValue(item))
            .filter(Boolean)
            .join(', ');
    }
    return '';
}

function buildAuditOverviewItems(record: AuditLog, t: AuditTranslation): DescriptionsProps['items'] {
    const auditIdItem = {
        key: 'auditId',
        label: t('audit.detail_label.audit_id', { defaultValue: 'Audit ID' }),
        children: (
            <Text copyable={{ text: record.id }} code>
                {shortAuditID(record.id)}
            </Text>
        ),
    };
    const resourceIdItem = {
        key: 'resourceId',
        label: t('audit.detail_label.resource_id', { defaultValue: 'Resource ID' }),
        children: (
            <Text copyable={{ text: record.resource_id }} code>
                {shortAuditID(record.resource_id)}
            </Text>
        ),
    };
    const timeItem = {
        key: 'time',
        label: t('audit.detail_label.time', { defaultValue: 'Occurred at' }),
        children: <LocalDateTimeText value={record.created_at} />,
    };

    if (record.resource_type === 'directory_sync_job') {
        return [
            {
                key: 'provider',
                label: t('audit.context.provider', { defaultValue: 'Auth provider' }),
                children: resourcePrimary(record),
            },
            {
                key: 'mode',
                label: t('audit.detail_label.mode', { defaultValue: 'Mode' }),
                children: record.resource_summary?.secondary
                    ? translateAuditDirectorySyncMode(t, record.resource_summary.secondary)
                    : '—',
            },
            {
                key: 'status',
                label: t('audit.detail_label.status', { defaultValue: 'Status' }),
                children: record.resource_summary?.tertiary
                    ? translateAuditDirectorySyncStatus(t, record.resource_summary.tertiary)
                    : '—',
            },
            {
                key: 'actor',
                label: t('audit.detail_label.triggered_by', { defaultValue: 'Triggered by' }),
                children: joinAuditParts(actorPrimary(record, t), actorSecondary(record, t)),
            },
            timeItem,
            auditIdItem,
            {
                ...resourceIdItem,
                key: 'jobId',
                label: t('audit.detail_label.job_id', { defaultValue: 'Job ID' }),
            },
        ];
    }

    if (isUserIdentityAuditRecord(record)) {
        return [
            {
                key: 'action',
                label: t('audit.detail_label.action', { defaultValue: 'Action' }),
                children: translateAuditAction(t, record.action),
            },
            {
                key: 'user',
                label: t('audit.detail_label.user', { defaultValue: 'User' }),
                children: resourcePrimary(record),
            },
            {
                key: 'identity',
                label: t('audit.detail_label.identity', { defaultValue: 'Identity' }),
                children: joinAuditParts(record.resource_summary?.secondary, actorSecondary(record, t)),
            },
            timeItem,
            auditIdItem,
        ];
    }

    if (record.resource_type === 'ticket') {
        const items: DescriptionsProps['items'] = [
            {
                key: 'action',
                label: t('audit.detail_label.action', { defaultValue: 'Action' }),
                children: translateAuditAction(t, record.action),
            },
            {
                key: 'requestedBy',
                label: t('audit.detail_label.requested_by', { defaultValue: 'Requested by' }),
                children: ticketRequesterLabel(record.ticket_summary) || '—',
            },
            {
                key: 'approver',
                label: t('audit.detail_label.approver', { defaultValue: 'Approver' }),
                children: ticketApproverLabel(record.ticket_summary) || '—',
            },
            {
                key: 'owner',
                label: t('audit.context.owner', { defaultValue: 'Owner' }),
                children: ticketOwnerLabel(record.ticket_summary) || '—',
            },
            timeItem,
            auditIdItem,
            {
                ...resourceIdItem,
                key: 'ticketId',
                label: t('audit.detail_label.ticket_id', { defaultValue: 'Ticket ID' }),
            },
        ];

        if (record.approval_decision) {
            items.splice(1, 0, {
                key: 'decision',
                label: t('audit.detail_label.decision', { defaultValue: 'Decision' }),
                children: translateAuditDecision(t, record.approval_decision),
            });
        }

        return items;
    }

    const items: DescriptionsProps['items'] = [
        {
            key: 'action',
            label: t('audit.detail_label.action', { defaultValue: 'Action' }),
            children: translateAuditAction(t, record.action),
        },
        {
            key: 'resource',
            label: t('audit.detail_label.resource', { defaultValue: 'Resource' }),
            children: resourcePrimary(record),
        },
        {
            key: 'resourceType',
            label: t('audit.detail_label.resource_type', { defaultValue: 'Resource type' }),
            children: translateAuditResourceType(t, record.resource_type),
        },
        {
            key: 'actor',
            label: t(
                record.resource_type === 'ticket'
                    ? 'audit.detail_label.requested_by'
                    : 'audit.detail_label.actor',
                { defaultValue: record.resource_type === 'ticket' ? 'Requested by' : 'Actor' },
            ),
            children: joinAuditParts(actorPrimary(record, t), actorSecondary(record, t), record.actor),
        },
        timeItem,
        auditIdItem,
        record.resource_type === 'ticket'
            ? {
                ...resourceIdItem,
                key: 'ticketId',
                label: t('audit.detail_label.ticket_id', { defaultValue: 'Ticket ID' }),
            }
            : resourceIdItem,
    ];

    if (record.approval_decision) {
        items.splice(1, 0, {
            key: 'decision',
            label: t('audit.detail_label.decision', { defaultValue: 'Decision' }),
            children: translateAuditDecision(t, record.approval_decision),
        });
    }

    return items;
}

function buildAuditTicketItems(summary: TicketSummary, t: AuditTranslation): DescriptionsProps['items'] {
    const items: DescriptionsProps['items'] = [];
    const push = (key: string, value?: string | number) => {
        const normalized = typeof value === 'string' ? value.trim() : value;
        if (normalized === '' || normalized === undefined || normalized === null || normalized === 0) {
            return;
        }
        items.push({
            key,
            label: translateAuditFieldLabel(t, key),
            children: String(normalized),
        });
    };

    push('system_name', summary.system_name);
    push('service_name', summary.service_name);
    push('namespace', summary.namespace);
    push('cluster_name', summary.cluster_name || summary.cluster_id);
    push('cluster_environment', summary.cluster_environment);
    push('owner_display_name', ticketOwnerLabel(summary));
    push('vm_name', summary.vm_name || summary.vm_id);
    push('template_name', summary.template_name || summary.template_id);
    push('instance_size_name', summary.instance_size_name || summary.instance_size_id);
    push('request_vm_status', summary.request_vm_status);
    push('latest_vm_status', summary.latest_vm_status);
    push('power_action', summary.power_action);
    push('batch_count', summary.batch_count);
    push('target_cpu_cores', summary.target_cpu_cores ? `${trimFloat(summary.target_cpu_cores)} vCPU` : '');
    push('target_memory_gi', summary.target_memory_gi ? `${trimFloat(summary.target_memory_gi)} Gi` : '');
    push('target_disk_gb', summary.target_disk_gb ? `${summary.target_disk_gb} Gi` : '');

    return items;
}

function buildAuditPlacementItems(summary: AuditPlacementSummary, t: AuditTranslation): DescriptionsProps['items'] {
    const items: DescriptionsProps['items'] = [];
    if (summary.selected_cluster_name || summary.selected_cluster_id) {
        items.push({
            key: 'selected_cluster',
            label: t('audit.detail_label.selected_cluster', { defaultValue: 'Selected cluster' }),
            children: summary.selected_cluster_name || summary.selected_cluster_id,
        });
    }
    if (summary.reason_code) {
        items.push({
            key: 'reason_code',
            label: t('audit.detail_label.reason_code', { defaultValue: 'Reason code' }),
            children: summary.reason_code,
        });
    }
    if (summary.advisory_code) {
        items.push({
            key: 'advisory_code',
            label: t('audit.detail_label.advisory_code', { defaultValue: 'Advisory code' }),
            children: summary.advisory_code,
        });
    }
    if (summary.eligible !== undefined && summary.eligible !== null) {
        items.push({
            key: 'eligible',
            label: t('audit.detail_label.eligible', { defaultValue: 'Eligibility' }),
            children: summary.eligible
                ? t('audit.placement.eligible', { defaultValue: 'Eligible' })
                : t('audit.placement.denied', { defaultValue: 'Denied' }),
        });
    }
    return items;
}

function buildAuditScalarDetailItems(
    details: Record<string, unknown> | undefined,
    t: AuditTranslation,
): DescriptionsProps['items'] {
    if (!details) {
        return [];
    }
    const ignoredKeys = new Set([
        'decision',
        'placement_evaluation',
        'request_snapshot',
        'batch_summary',
    ]);

    return Object.entries(details)
        .filter(([key, value]) => !ignoredKeys.has(key) && typeof value !== 'object')
        .map(([key, value]) => ({
            key,
            label: translateAuditFieldLabel(t, key),
            children: formatAuditValue(value) || '—',
        }))
        .filter((item) => item.children !== '—');
}

function buildAuditNestedDetailSections(
    details: Record<string, unknown> | undefined,
    t: AuditTranslation,
): Array<{ key: string; title: string; items: DescriptionsProps['items'] }> {
    if (!details) {
        return [];
    }
    return Object.entries(details)
        .filter(([, value]) => value && typeof value === 'object' && !Array.isArray(value))
        .flatMap(([key, value]) => {
            if (!value || Array.isArray(value)) {
                return [];
            }
            const entries = Object.entries(value as Record<string, unknown>)
                .map(([childKey, childValue]) => ({
                    key: `${key}.${childKey}`,
                    label: translateAuditFieldLabel(t, childKey),
                    children: formatAuditValue(childValue) || '—',
                }))
                .filter((item) => item.children !== '—');
            if (entries.length === 0) {
                return [];
            }
            return [{
                key,
                title: translateAuditFieldLabel(t, key),
                items: entries,
            }];
        });
}

export function AdminAuditContent() {
    const { t } = useTranslation(['admin', 'common']);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [selectedLog, setSelectedLog] = useState<AuditLog | null>(null);
    const [filters, setFilters] = useState<AuditLogFilters>({
        search: '',
        category: '',
        action: '',
        approval_decision: '',
        actor: '',
        placement_advisory_code: '',
        placement_reason_code: '',
        resource_type: '',
        resource_id: '',
    });
    const [draftFilters, setDraftFilters] = useState<AuditLogFilters>({
        search: '',
        category: '',
        action: '',
        approval_decision: '',
        actor: '',
        placement_advisory_code: '',
        placement_reason_code: '',
        resource_type: '',
        resource_id: '',
    });
    const [advancedOpen, setAdvancedOpen] = useState(false);
    const resourceTypeOptions = [
        { label: t('audit.resource_option.all'), value: '' },
        { label: t('audit.resource_option.vm'), value: 'vm' },
        { label: t('audit.resource_option.system'), value: 'system' },
        { label: t('audit.resource_option.service'), value: 'service' },
        { label: t('audit.resource_option.ticket'), value: 'ticket' },
        { label: t('audit.resource_option.cluster'), value: 'cluster' },
        { label: t('audit.resource_option.user'), value: 'user' },
        { label: t('audit.resource_option.namespace'), value: 'namespace' },
        { label: t('audit.resource_option.template'), value: 'template' },
        { label: t('audit.resource_option.instance_size'), value: 'instance_size' },
        { label: t('audit.resource_option.role'), value: 'role' },
        { label: t('audit.resource_option.auth_provider'), value: 'auth_provider' },
        { label: t('audit.resource_option.directory_sync_job'), value: 'directory_sync_job' },
    ];
    const approvalDecisionOptions = [
        { label: t('audit.decision_option.all'), value: '' },
        { label: t('audit.decision_option.approved'), value: 'approved' },
        { label: t('audit.decision_option.rejected'), value: 'rejected' },
        { label: t('audit.decision_option.validation_failed'), value: 'validation_failed' },
        { label: t('audit.decision_option.power_approved'), value: 'power_approved' },
        { label: t('audit.decision_option.delete_approved'), value: 'delete_approved' },
        { label: t('audit.decision_option.vnc_access_approved'), value: 'vnc_access_approved' },
        { label: t('audit.decision_option.batch_approved'), value: 'batch_approved' },
        { label: t('audit.decision_option.batch_rejected'), value: 'batch_rejected' },
        { label: t('audit.decision_option.cancelled'), value: 'cancelled' },
        { label: t('audit.decision_option.batch_cancelled'), value: 'batch_cancelled' },
    ];

    const { data, isLoading, refetch } = useApiGet<AuditLogList>(
        ['audit-logs', page, pageSize, filters],
        () =>
            api.GET('/audit-logs', {
                params: {
                    query: buildAuditLogQuery(page, pageSize, filters),
                },
            }),
    );
    const auditItems = useMemo(() => data?.items ?? [], [data?.items]);
    const actionOptions = useMemo(
        () =>
            dedupeSortedStrings([...COMMON_AUDIT_ACTIONS, ...auditItems.map((item) => item.action)]).map((value) => ({
                value,
                label: translateAuditAction(t, value),
            })),
        [auditItems, t],
    );
    const actorOptions = useMemo(
        () =>
            dedupeSortedStrings(auditItems.map((item) => item.actor)).map((value) => {
                const item = auditItems.find((candidate) => candidate.actor === value);
                return {
                    value,
                    label: joinAuditParts(
                        item?.actor_summary?.display_name,
                        item?.actor_summary?.secondary,
                        value,
                    ),
                };
            }),
        [auditItems],
    );
    const resourceIdOptions = useMemo(
        () =>
            dedupeSortedStrings(auditItems.map((item) => item.resource_id)).map((value) => {
                const item = auditItems.find((candidate) => candidate.resource_id === value);
                return {
                    value,
                    label: joinAuditParts(
                        item?.resource_summary?.display_name,
                        value,
                    ),
                };
            }),
        [auditItems],
    );
    const placementAdvisoryOptions = useMemo(
        () =>
            dedupeSortedStrings(
                auditItems.map((item) => item.placement_summary?.advisory_code),
            ).map((value) => ({
                value,
                label:
                    value === 'PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY'
                        ? `${t('approval:filter.placement_advisory_host_assisted_clone', {
                              defaultValue: 'Host-assisted clone fallback likely',
                          })} · ${value}`
                        : value,
            })),
        [auditItems, t],
    );
    const placementReasonOptions = useMemo(
        () =>
            dedupeSortedStrings(
                auditItems.map((item) => item.placement_summary?.reason_code),
            ).map((value) => ({
                value,
                label: value,
            })),
        [auditItems],
    );
    const actorsVisible = new Set(auditItems.map((item) => item.actor).filter(Boolean)).size;
    const decisionsVisible = auditItems.filter((item) => Boolean(item.approval_decision)).length;
    const placementVisible = auditItems.filter((item) => Boolean(item.placement_summary)).length;
    const presetOptions: Array<{ key: AuditPresetKey; label: string }> = useMemo(
        () => [
            { key: '', label: t('audit.preset.all', { defaultValue: 'All activity' }) },
            { key: 'requests', label: t('audit.preset.requests', { defaultValue: 'Requests' }) },
            { key: 'approvals', label: t('audit.preset.approvals', { defaultValue: 'Approvals' }) },
            { key: 'resource_changes', label: t('audit.preset.resource_changes', { defaultValue: 'Resource changes' }) },
            { key: 'system_tasks', label: t('audit.preset.system_tasks', { defaultValue: 'System tasks' }) },
        ],
        [t],
    );
    const hasAdvancedFilters = useMemo(
        () => [
            filters.category,
            filters.resource_type,
            filters.action,
            filters.approval_decision,
            filters.actor,
            filters.resource_id,
            filters.placement_advisory_code,
            filters.placement_reason_code,
        ].some((value) => value.trim() !== ''),
        [filters],
    );
    const hasActiveFilters = useMemo(
        () => Object.values(filters).some((value) => value.trim() !== ''),
        [filters],
    );

    const applyFilters = (next: AuditLogFilters) => {
        setPage(1);
        setFilters(normalizeAuditFilters(next));
    };

    const resetFilters = () => {
        const empty: AuditLogFilters = {
            search: '',
            category: '',
            action: '',
            approval_decision: '',
            actor: '',
            placement_advisory_code: '',
            placement_reason_code: '',
            resource_type: '',
            resource_id: '',
        };
        setPage(1);
        setFilters(empty);
        setDraftFilters(empty);
        setAdvancedOpen(false);
    };

    const columns: ColumnsType<AuditLog> = [
        {
            title: t('audit.feed', { defaultValue: 'Activity feed' }),
            key: 'feed',
            width: 420,
            render: (_: unknown, record: AuditLog) => (
                <div className={`audit-feed-card${isLowSignalAuditRecord(record) ? ' audit-feed-card--muted' : ''}`}>
                    <Space size={4} wrap className="audit-feed-card__eyebrow">
                        {buildAuditFeedBadges(record, t).map((badge) => (
                            <Tag key={badge.key} color={badge.color ?? 'default'}>
                                {badge.label}
                            </Tag>
                        ))}
                    </Space>
                    <Text strong className="audit-feed-card__headline">
                        {buildAuditHeadline(record, t)}
                    </Text>
                    <Flex gap={6} wrap className="audit-feed-card__meta">
                        {buildAuditFeedMeta(record, t).map((part) => (
                            <Text key={part} type="secondary" className="audit-feed-card__meta-pill">
                                {part}
                            </Text>
                        ))}
                    </Flex>
                </div>
            ),
        },
        {
            title: t('audit.context_title', { defaultValue: 'Context' }),
            key: 'context',
            width: 360,
            render: (_: unknown, record: AuditLog) => {
                const rows = buildContextRows(record, t);
                if (rows.length === 0) {
                    return <Text type="secondary">—</Text>;
                }
                return (
                    <div className="audit-context-list">
                        {rows.map((row) => (
                            <div key={row.key} className="audit-context-list__row">
                                <Text type="secondary" className="audit-context-list__label">
                                    {row.label}
                                </Text>
                                <Text className="audit-context-list__value">{row.value}</Text>
                            </div>
                        ))}
                    </div>
                );
            },
        },
        {
            title: t('audit.timestamp'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 170,
            render: (date: string) => <LocalDateTimeText value={date} />,
        },
        {
            title: t('common:table.actions', { defaultValue: 'Actions' }),
            key: 'actions',
            width: 120,
            render: (_: unknown, record: AuditLog) => (
                <Space className="copy-friendly-actions">
                    <Button
                        aria-label={t('audit.view_details', { defaultValue: 'Details' })}
                        icon={<EyeOutlined />}
                        onClick={() => setSelectedLog(record)}
                    >
                        {t('audit.view_details', { defaultValue: 'Details' })}
                    </Button>
                </Space>
            ),
        },
    ];

    const scalarDetailItems: NonNullable<DescriptionsProps['items']> = selectedLog
        ? (buildAuditScalarDetailItems(selectedLog.details, t) ?? [])
        : [];
    const nestedDetailSections = selectedLog ? buildAuditNestedDetailSections(selectedLog.details, t) : [];
    const overviewItems = selectedLog ? buildAuditOverviewItems(selectedLog, t) : [];
    const ticketItems: NonNullable<DescriptionsProps['items']> = selectedLog?.ticket_summary
        ? (buildAuditTicketItems(selectedLog.ticket_summary, t) ?? [])
        : [];
    const batchItemCards = selectedLog?.ticket_summary
        ? buildTicketBatchItemCards(selectedLog.ticket_summary, t)
        : [];
    const placementItems: NonNullable<DescriptionsProps['items']> = selectedLog?.placement_summary
        ? (buildAuditPlacementItems(selectedLog.placement_summary, t) ?? [])
        : [];
    const hasDiagnosticDetails = scalarDetailItems.length > 0 || nestedDetailSections.length > 0;

    return (
        <div className="audit-page copy-friendly-actions">
            <PageHeader
                title={t('audit.title')}
                subtitle={t('audit.subtitle')}
                actions={(
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                )}
            />
            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('audit.summary.total_title', { defaultValue: 'Visible events' })}
                    value={auditItems.length}
                    description={t('audit.summary.total_description', { defaultValue: 'Audit entries visible with the current filters.' })}
                    visual={<NotificationInboxGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('audit.summary.decisions_title', { defaultValue: 'Decision traces' })}
                    value={decisionsVisible}
                    description={t('audit.summary.decisions_description', { defaultValue: 'Entries that include an approval outcome.' })}
                    visual={<RequestsOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('audit.summary.placement_title', { defaultValue: 'Placement reviews' })}
                    value={placementVisible}
                    description={t('audit.summary.placement_description', { defaultValue: 'Entries carrying placement evaluation context.' })}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#D66A1F"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('audit.summary.actors_title', { defaultValue: 'Active actors' })}
                    value={actorsVisible}
                    description={t('audit.summary.actors_description', { defaultValue: 'Distinct actors represented in this result set.' })}
                    visual={<ServiceWorkspaceGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface style={{ marginBottom: 16 }}>
                <Flex vertical gap={12} style={{ padding: '16px 16px 0' }}>
                    <Flex wrap gap={8} align="center" className="audit-preset-bar copy-friendly-actions">
                        <Text type="secondary" className="audit-preset-bar__label">
                            {t('audit.preset.label', { defaultValue: 'Quick views' })}
                        </Text>
                        {presetOptions.map((preset) => {
                            const active = filters.category === preset.key;
                            return (
                                <Button
                                    key={preset.key || 'all'}
                                    type={active ? 'primary' : 'default'}
                                    size="small"
                                    className={active ? 'audit-preset-button audit-preset-button--active' : 'audit-preset-button'}
                                    onClick={() => {
                                        const next = { ...draftFilters, category: preset.key };
                                        setDraftFilters(next);
                                        applyFilters(next);
                                    }}
                                >
                                    {preset.label}
                                </Button>
                            );
                        })}
                    </Flex>
                </Flex>
                <PageSearchToolbar
                    searchValue={filters.search}
                    searchDraftValue={draftFilters.search}
                    onSearchDraftChange={(value) => setDraftFilters((current) => ({ ...current, search: value }))}
                    onSearchChange={(value) => {
                        applyFilters({ ...draftFilters, search: value });
                        setDraftFilters((current) => ({ ...current, search: value }));
                    }}
                    searchPlaceholder={t('audit.search_placeholder', {
                        defaultValue: 'Search action, actor, resource type, or resource ID',
                    })}
                    searchHelp={t('audit.search_help', {
                        defaultValue: 'Quick search matches action, actor, resource type, and resource ID. Use advanced search for audit-specific fields.',
                    })}
                    searchTestId="audit-search-input"
                    hasActiveFilters={hasActiveFilters}
                    onClear={resetFilters}
                    clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                    clearTestId="audit-clear-filters"
                    secondaryActions={(
                        <Space className="copy-friendly-actions">
                            <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                                {t('common:button.refresh')}
                            </Button>
                        </Space>
                    )}
                    advancedSearch={{
                        open: advancedOpen,
                        onToggle: () => setAdvancedOpen((current) => !current),
                        openLabel: t('common:search.advanced'),
                        closeLabel: t('common:search.hide_advanced'),
                        title: t('audit.advanced_search_title', {
                            defaultValue: 'Advanced search',
                        }),
                        toggleTestId: 'audit-advanced-search-toggle',
                        content: (
                            <Flex vertical gap={12}>
                                <Flex wrap gap={12}>
                                    <Select
                                        style={{ flex: '1 1 220px', minWidth: 220 }}
                                        placeholder={t('audit.filter.resource_type')}
                                        showSearch
                                        optionFilterProp="label"
                                        value={draftFilters.resource_type || undefined}
                                        onChange={(value) => setDraftFilters((current) => ({
                                            ...current,
                                            resource_type: value || '',
                                        }))}
                                        options={resourceTypeOptions}
                                        allowClear
                                    />
                                    <Select
                                        style={{ flex: '1 1 220px', minWidth: 220 }}
                                        placeholder={t('audit.filter.action')}
                                        showSearch
                                        optionFilterProp="label"
                                        value={draftFilters.action || undefined}
                                        onChange={(value) => setDraftFilters((current) => ({
                                            ...current,
                                            action: value || '',
                                        }))}
                                        options={actionOptions}
                                        allowClear
                                    />
                                    <Select
                                        style={{ flex: '1 1 220px', minWidth: 220 }}
                                        placeholder={t('audit.filter.approval_decision')}
                                        showSearch
                                        optionFilterProp="label"
                                        value={draftFilters.approval_decision || undefined}
                                        onChange={(value) => setDraftFilters((current) => ({
                                            ...current,
                                            approval_decision: value || '',
                                        }))}
                                        options={approvalDecisionOptions}
                                        allowClear
                                    />
                                    <AutoComplete
                                        style={{ flex: '1 1 220px', minWidth: 220 }}
                                        placeholder={t('audit.filter.actor')}
                                        value={draftFilters.actor}
                                        options={actorOptions}
                                        filterOption={(inputValue, option) =>
                                            String(option?.label ?? '')
                                                .toLowerCase()
                                                .includes(inputValue.trim().toLowerCase())
                                        }
                                        onChange={(value) => setDraftFilters((current) => ({
                                            ...current,
                                            actor: value,
                                        }))}
                                        allowClear
                                    />
                                    <AutoComplete
                                        style={{ flex: '1 1 220px', minWidth: 220 }}
                                        placeholder={t('audit.filter.resource_id', {
                                            defaultValue: 'Filter by Resource ID',
                                        })}
                                        value={draftFilters.resource_id}
                                        options={resourceIdOptions}
                                        filterOption={(inputValue, option) =>
                                            String(option?.label ?? '')
                                                .toLowerCase()
                                                .includes(inputValue.trim().toLowerCase())
                                        }
                                        onChange={(value) => setDraftFilters((current) => ({
                                            ...current,
                                            resource_id: value,
                                        }))}
                                        allowClear
                                    />
                                    <Select
                                        style={{ flex: '1 1 220px', minWidth: 220 }}
                                        placeholder={t('audit.filter.placement_advisory_code')}
                                        showSearch
                                        optionFilterProp="label"
                                        value={draftFilters.placement_advisory_code || undefined}
                                        onChange={(value) => setDraftFilters((current) => ({
                                            ...current,
                                            placement_advisory_code: value || '',
                                        }))}
                                        options={placementAdvisoryOptions}
                                        allowClear
                                    />
                                    <Select
                                        style={{ flex: '1 1 220px', minWidth: 220 }}
                                        placeholder={t('audit.filter.placement_reason_code')}
                                        showSearch
                                        optionFilterProp="label"
                                        value={draftFilters.placement_reason_code || undefined}
                                        onChange={(value) => setDraftFilters((current) => ({
                                            ...current,
                                            placement_reason_code: value || '',
                                        }))}
                                        options={placementReasonOptions}
                                        allowClear
                                    />
                                </Flex>
                                <Flex justify="space-between" align="center" gap={12} wrap>
                                    <Text type="secondary" style={{ fontSize: 12 }}>
                                        {t('audit.advanced_search_help', {
                                            defaultValue: 'Use advanced search for approval decisions, placement diagnostics, and exact resource context.',
                                        })}
                                    </Text>
                                    <Button
                                        type="primary"
                                        onClick={() => {
                                            applyFilters(draftFilters);
                                            if (!hasAdvancedFilters) {
                                                setAdvancedOpen(true);
                                            }
                                        }}
                                    >
                                        {t('common:button.search')}
                                    </Button>
                                </Flex>
                            </Flex>
                        ),
                    }}
                />
            </PageSurface>

            <PageSurface flush={true}>
                <Table<AuditLog>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    rowClassName={(record) => (
                        isLowSignalAuditRecord(record) ? 'audit-table__row audit-table__row--muted' : 'audit-table__row'
                    )}
                    pagination={{
                        current: page,
                        pageSize,
                        total: data?.pagination?.total ?? 0,
                        showTotal: (total) => t('common:table.total', { total }),
                        onChange: (p, ps) => { setPage(p); setPageSize(ps); },
                    }}
                    size="middle"
                    scroll={{ x: 'max-content' }}
                    locale={{
                        emptyText: (
                            <ActionEmptyState
                                compact={true}
                                title={t('audit.empty', { defaultValue: 'No audit activity' })}
                                description={t('audit.empty_description', { defaultValue: 'Try a broader filter, or return later after new platform activity is recorded.' })}
                                visual={<NotificationInboxGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                            />
                        ),
                    }}
                />
            </PageSurface>

            <Drawer
                width={560}
                open={Boolean(selectedLog)}
                onClose={() => setSelectedLog(null)}
                title={t('audit.drawer_title', { defaultValue: 'Audit event details' })}
                destroyOnClose
            >
                {selectedLog ? (
                    <Space direction="vertical" size={16} style={{ width: '100%' }}>
                        <div className="audit-drawer-hero">
                            <Space size={4} wrap className="audit-feed-card__eyebrow">
                                {buildAuditFeedBadges(selectedLog, t).map((badge) => (
                                    <Tag key={badge.key} color={badge.color ?? 'default'}>
                                        {badge.label}
                                    </Tag>
                                ))}
                            </Space>
                            <Title level={5} style={{ margin: 0 }}>
                                {buildAuditHeadline(selectedLog, t)}
                            </Title>
                            <Flex gap={6} wrap className="audit-drawer-hero__meta">
                                {buildAuditFeedMeta(selectedLog, t).map((part) => (
                                    <Text key={part} type="secondary" className="audit-feed-card__meta-pill">
                                        {part}
                                    </Text>
                                ))}
                            </Flex>
                        </div>

                        <div className="audit-detail-section">
                            <Text strong className="audit-detail-section__title">
                                {t('audit.section.overview', { defaultValue: 'Overview' })}
                            </Text>
                            <Descriptions size="small" column={1} items={overviewItems} />
                        </div>

                        {ticketItems.length > 0 ? (
                            <div className="audit-detail-section">
                                <Text strong className="audit-detail-section__title">
                                    {t('audit.section.ticket_context', { defaultValue: 'Request context' })}
                                </Text>
                                <Descriptions size="small" column={1} items={ticketItems} />
                            </div>
                        ) : null}

                        {batchItemCards.length > 0 ? (
                            <div className="audit-detail-section">
                                <Text strong className="audit-detail-section__title">
                                    {t('audit.section.batch_items', { defaultValue: 'Batch items' })}
                                </Text>
                                <div className="audit-batch-item-grid">
                                    {batchItemCards.map((item) => (
                                        <div key={item.key} className="audit-batch-item-card">
                                            <Text strong className="audit-batch-item-card__title">
                                                {item.title}
                                            </Text>
                                            {item.subtitle ? (
                                                <Text type="secondary" className="audit-batch-item-card__meta">
                                                    {item.subtitle}
                                                </Text>
                                            ) : null}
                                            {item.lines.map((line) => (
                                                <div key={line.label} className="audit-batch-item-card__line">
                                                    <Text type="secondary" className="audit-batch-item-card__line-label">
                                                        {t(line.label, { defaultValue: line.label })}
                                                    </Text>
                                                    <Text className="audit-batch-item-card__line-value">
                                                        {line.value}
                                                    </Text>
                                                </div>
                                            ))}
                                        </div>
                                    ))}
                                </div>
                            </div>
                        ) : null}

                        {placementItems.length > 0 ? (
                            <div className="audit-detail-section">
                                <Text strong className="audit-detail-section__title">
                                    {t('audit.section.placement', { defaultValue: 'Placement review' })}
                                </Text>
                                <Descriptions size="small" column={1} items={placementItems} />
                            </div>
                        ) : null}

                        {hasDiagnosticDetails ? (
                            <Collapse
                                size="small"
                                ghost
                                items={[
                                    {
                                        key: 'diagnostic-details',
                                        label: t('audit.section.diagnostics', { defaultValue: 'Diagnostic details' }),
                                        children: (
                                            <Space direction="vertical" size={16} style={{ width: '100%' }}>
                                                {scalarDetailItems.length > 0 ? (
                                                    <div className="audit-detail-section audit-detail-section--nested">
                                                        <Text strong className="audit-detail-section__title">
                                                            {t('audit.section.attributes', { defaultValue: 'Detail attributes' })}
                                                        </Text>
                                                        <Descriptions size="small" column={1} items={scalarDetailItems} />
                                                    </div>
                                                ) : null}
                                                {nestedDetailSections.map((section) => (
                                                    <div key={section.key} className="audit-detail-section audit-detail-section--nested">
                                                        <Text strong className="audit-detail-section__title">
                                                            {section.title}
                                                        </Text>
                                                        <Descriptions size="small" column={1} items={section.items} />
                                                    </div>
                                                ))}
                                            </Space>
                                        ),
                                    },
                                ]}
                            />
                        ) : null}

                        {selectedLog.details && Object.keys(selectedLog.details).length > 0 ? (
                            <Collapse
                                size="small"
                                ghost
                                items={[
                                    {
                                        key: 'raw-json',
                                        label: t('audit.details.raw_json', { defaultValue: 'Raw JSON' }),
                                        children: (
                                            <pre className="audit-raw-json">
                                                {JSON.stringify(selectedLog.details, null, 2)}
                                            </pre>
                                        ),
                                    },
                                ]}
                            />
                        ) : null}
                    </Space>
                ) : null}
            </Drawer>
        </div>
    );
}
