'use client';

import { useMemo, useState } from 'react';
import {
    AutoComplete,
    Button,
    Badge,
    Flex,
    Popover,
    Select,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined } from '@ant-design/icons';
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

const { Text } = Typography;

type AuditLog = components['schemas']['AuditLog'];
type AuditLogList = components['schemas']['AuditLogList'];
type AuditPlacementSummary = components['schemas']['AuditPlacementSummary'];

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

function normalizeActionKey(action?: string): string {
    return (action ?? '').trim().toLowerCase().replace(/[.\s-]+/g, '_');
}

function actionSuffix(action?: string): string {
    const normalized = normalizeActionKey(action);
    const tokens = normalized.split('_').filter(Boolean);
    return tokens.at(-1) ?? normalized;
}

function placementStatusTagColor(summary: AuditPlacementSummary): string {
    if (summary.eligible === true) {
        return 'success';
    }
    if (summary.eligible === false) {
        return 'error';
    }
    return 'default';
}

function normalizeAuditFilters(filters: AuditLogFilters): AuditLogFilters {
    return {
        search: filters.search.trim(),
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

export function AdminAuditContent() {
    const { t } = useTranslation(['admin', 'common']);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [filters, setFilters] = useState<AuditLogFilters>({
        search: '',
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
            })
    );
    const auditItems = useMemo(() => data?.items ?? [], [data?.items]);
    const actionOptions = useMemo(
        () =>
            dedupeSortedStrings(auditItems.map((item) => item.action)).map((value) => ({
                value,
                label: t(`audit.action_code.${normalizeActionKey(value)}`, {
                    defaultValue: value,
                }),
            })),
        [auditItems, t],
    );
    const actorOptions = useMemo(
        () =>
            dedupeSortedStrings(auditItems.map((item) => item.actor)).map((value) => ({
                value,
                label: value,
            })),
        [auditItems],
    );
    const resourceIdOptions = useMemo(
        () =>
            dedupeSortedStrings(auditItems.map((item) => item.resource_id)).map((value) => ({
                value,
                label: value,
            })),
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
    const hasAdvancedFilters = useMemo(
        () => [
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
            title: t('audit.action'),
            dataIndex: 'action',
            key: 'action',
            width: 130,
            render: (action: string) => {
                const normalized = normalizeActionKey(action);
                const normalizedLabel = t(`audit.action_code.${normalized}`, { defaultValue: '' });
                const suffix = actionSuffix(action);
                const fallbackLabel = t(`audit.action_code.${suffix}`, { defaultValue: action?.toUpperCase() ?? '—' });
                const label = normalizedLabel || fallbackLabel;
                return (
                    <Tag color={ACTION_COLORS[suffix.toUpperCase()] ?? ACTION_COLORS[action?.toUpperCase()] ?? 'default'}>
                        {label}
                    </Tag>
                );
            },
        },
        {
            title: t('audit.decision', { defaultValue: 'Decision' }),
            dataIndex: 'approval_decision',
            key: 'decision',
            width: 160,
            render: (decision?: string) => {
                if (!decision) {
                    return <Text type="secondary">—</Text>;
                }
                return (
                    <Tag color={DECISION_COLORS[decision] ?? 'blue'}>
                        {t(`audit.decision_option.${decision}`, { defaultValue: decision })}
                    </Tag>
                );
            },
        },
        {
            title: t('audit.actor'),
            dataIndex: 'actor',
            key: 'actor',
            width: 150,
            render: (actor: string) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{actor || '—'}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                        {t('audit.actor_label', { defaultValue: 'Actor identity' })}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('audit.resource', { defaultValue: 'Resource' }),
            key: 'resource',
            width: 220,
            render: (_: unknown, record: AuditLog) => (
                <Space direction="vertical" size={2}>
                    <Badge
                        status="processing"
                        text={t(`audit.resource_option.${(record.resource_type ?? '').toLowerCase()}`, { defaultValue: record.resource_type })}
                    />
                    <Text copyable style={{ fontSize: 12 }}>
                        {record.resource_id || '—'}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('audit.placement', { defaultValue: 'Placement' }),
            dataIndex: 'placement_summary',
            key: 'placement',
            width: 280,
            render: (summary?: AuditPlacementSummary) => {
                if (!summary) {
                    return <Text type="secondary">—</Text>;
                }
                return (
                    <Space direction="vertical" size={2}>
                        <Space wrap size={4}>
                            <Tag color={placementStatusTagColor(summary)}>
                                {summary.eligible === true
                                    ? t('audit.placement.eligible', { defaultValue: 'Eligible' })
                                    : summary.eligible === false
                                        ? t('audit.placement.denied', { defaultValue: 'Denied' })
                                        : '—'}
                            </Tag>
                            {summary.reason_code && (
                                <Tag color="red">{summary.reason_code}</Tag>
                            )}
                            {summary.advisory_code && (
                                <Tag color="orange">{summary.advisory_code}</Tag>
                            )}
                        </Space>
                        {(summary.selected_cluster_name || summary.selected_cluster_id) && (
                            <Text type="secondary" style={{ fontSize: 12 }}>
                                {t('audit.placement.cluster', { defaultValue: 'Cluster' })}: {summary.selected_cluster_name ?? summary.selected_cluster_id}
                            </Text>
                        )}
                    </Space>
                );
            },
        },
        {
            title: t('audit.details'),
            dataIndex: 'details',
            key: 'details',
            ellipsis: true,
            render: (details: Record<string, unknown> | undefined) => {
                if (!details || Object.keys(details).length === 0) return <Text type="secondary">—</Text>;
                return (
                    <Popover
                        content={
                            <div style={{ maxWidth: 420 }}>
                                <Text type="secondary" style={{ fontSize: 12 }}>
                                    {t('audit.details.raw_json', { defaultValue: 'Raw JSON' })}
                                </Text>
                                <pre style={{ maxWidth: 400, maxHeight: 300, overflow: 'auto', fontSize: 12, marginTop: 8 }}>
                                    {JSON.stringify(details, null, 2)}
                                </pre>
                            </div>
                        }
                        title={t('audit.details')}
                        trigger="click"
                    >
                        <Text code style={{ cursor: 'pointer', fontSize: 12 }}>
                            {'{...}'}
                        </Text>
                    </Popover>
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
    ];

    return (
        <div>
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
                    clearLabel={t('common:button.clear')}
                    clearTestId="audit-clear-filters"
                    secondaryActions={(
                        <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                            {t('common:button.refresh')}
                        </Button>
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
        </div>
    );
}
