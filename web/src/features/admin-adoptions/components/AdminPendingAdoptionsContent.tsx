'use client';

import {
    Button,
    Input,
    Modal,
    Select,
    Space,
    Table,
    Tag,
    Tooltip,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    CheckCircleOutlined,
    CloseCircleOutlined,
    CloudSyncOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    HealthOverviewGlyph,
    NotificationInboxGlyph,
    QueueReviewGlyph,
    SystemsOverviewGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import {
    PageSearchToolbar,
    filterOptionByLabel,
} from '@/components/ui/PageSearchToolbar';
import {
    PLATFORM_ADMIN_PERMISSION,
    hasPermission,
} from '@/lib/auth/permissions';
import { useAuthStore } from '@/stores/auth';

import { useAdminAdoptionsController } from '../hooks/useAdminAdoptionsController';
import {
    PENDING_ADOPTION_PAGE_SIZE,
    PENDING_ADOPTION_STATUS_COLORS,
    PENDING_ADOPTION_STATUS_OPTIONS,
    type PendingAdoption,
    type PendingAdoptionStatus,
} from '../types';

const { Text, Paragraph } = Typography;

const TRACKED_LABELS = [
    'shepherd.io/service-id',
    'shepherd.io/template-id',
    'shepherd.io/event-id',
];

function statusLabelKey(status: PendingAdoptionStatus) {
    return `adoptions.status.${status}`;
}

function renderLabels(labels: PendingAdoption['labels']) {
    const entries = Object.entries(labels ?? {});
    if (entries.length === 0) {
        return '—';
    }
    const prioritized = [
        ...entries.filter(([key]) => TRACKED_LABELS.includes(key)),
        ...entries.filter(([key]) => !TRACKED_LABELS.includes(key)),
    ];
    const visible = prioritized.slice(0, 3);
    const hiddenCount = Math.max(prioritized.length - visible.length, 0);
    return (
        <Space size={4} wrap>
            {visible.map(([key, value]) => (
                <Tooltip key={key} title={`${key}=${value}`}>
                    <Tag>
                        {key.replace('shepherd.io/', '')}: {value}
                    </Tag>
                </Tooltip>
            ))}
            {hiddenCount > 0 ? <Tag>+{hiddenCount}</Tag> : null}
        </Space>
    );
}

export function AdminPendingAdoptionsContent() {
    const { t } = useTranslation(['admin', 'common']);
    const user = useAuthStore((state) => state.user);
    const canDecideAdoptions = hasPermission(user, PLATFORM_ADMIN_PERMISSION);
    const adoptions = useAdminAdoptionsController({ t });
    const [filtersOpen, setFiltersOpen] = useState(false);
    const [quickSearchDraft, setQuickSearchDraft] = useState('');
    const [statusFilterDraft, setStatusFilterDraft] = useState<PendingAdoptionStatus | ''>('PENDING');
    const [clusterFilterDraft, setClusterFilterDraft] = useState('');
    const [namespaceFilterDraft, setNamespaceFilterDraft] = useState('');

    const adoptionItems = useMemo(
        () => adoptions.data?.items ?? [],
        [adoptions.data?.items],
    );
    const adoptionSummary = useMemo(() => ({
        visible: adoptionItems.length,
        pending: adoptionItems.filter((item) => item.status === 'PENDING').length,
        adopted: adoptionItems.filter((item) => item.status === 'ADOPTED').length,
        rejected: adoptionItems.filter((item) => item.status === 'REJECTED').length,
    }), [adoptionItems]);

    const statusOptions = PENDING_ADOPTION_STATUS_OPTIONS.map((status) => ({
        value: status,
        label: t(statusLabelKey(status), status),
    }));

    const columns: ColumnsType<PendingAdoption> = [
        {
            title: t('adoptions.table.resource', 'Resource'),
            dataIndex: 'resource_name',
            key: 'resource_name',
            width: 260,
            minWidth: 240,
            render: (resourceName: string, record) => (
                <Space size={8} align="start">
                    <CloudSyncOutlined style={{ color: '#1677ff' }} />
                    <div style={{ minWidth: 0 }}>
                        <Text strong ellipsis={{ tooltip: resourceName }}>
                            {resourceName}
                        </Text>
                        <br />
                        <Space size={4} wrap>
                            <Tag color="blue">{record.resource_type}</Tag>
                            <Text type="secondary" style={{ fontSize: 13 }}>
                                {record.id}
                            </Text>
                        </Space>
                    </div>
                </Space>
            ),
        },
        {
            title: t('common:table.status'),
            dataIndex: 'status',
            key: 'status',
            width: 130,
            render: (status: PendingAdoptionStatus) => (
                <Tag color={PENDING_ADOPTION_STATUS_COLORS[status]}>
                    {t(statusLabelKey(status), status)}
                </Tag>
            ),
        },
        {
            title: t('adoptions.table.location', 'Location'),
            key: 'location',
            width: 220,
            render: (_, record) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{record.namespace}</Text>
                    <Text type="secondary" style={{ fontSize: 13 }}>
                        {record.cluster_id}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('adoptions.table.labels', 'Labels'),
            dataIndex: 'labels',
            key: 'labels',
            width: 260,
            render: renderLabels,
        },
        {
            title: t('adoptions.table.discovered', 'Discovered'),
            key: 'discovered',
            width: 220,
            render: (_, record) => (
                <Space direction="vertical" size={0}>
                    <Text>{record.discovered_by || '—'}</Text>
                    <Text type="secondary" style={{ fontSize: 13 }}>
                        <LocalDateTimeText value={record.updated_at} />
                    </Text>
                </Space>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 180,
            fixed: 'right',
            render: (_, record) => {
                if (record.status !== 'PENDING' || !canDecideAdoptions) {
                    return <Text type="secondary">—</Text>;
                }
                return (
                    <Space size={4} wrap>
                        <Button
                            type="link"
                            size="small"
                            icon={<CheckCircleOutlined />}
                            data-testid={`adoption-action-adopt-${record.id}`}
                            onClick={() => adoptions.openDecision('adopt', record)}
                        >
                            {t('adoptions.action.adopt', 'Adopt')}
                        </Button>
                        <Button
                            type="link"
                            size="small"
                            danger
                            icon={<CloseCircleOutlined />}
                            data-testid={`adoption-action-reject-${record.id}`}
                            onClick={() => adoptions.openDecision('reject', record)}
                        >
                            {t('common:button.reject')}
                        </Button>
                    </Space>
                );
            },
        },
    ];

    const applyAdvancedFilters = () => {
        adoptions.changeSearch(quickSearchDraft);
        adoptions.changeStatusFilter(statusFilterDraft);
        adoptions.changeClusterFilter(clusterFilterDraft);
        adoptions.changeNamespaceFilter(namespaceFilterDraft);
    };

    const clearFilters = () => {
        setQuickSearchDraft('');
        setStatusFilterDraft('PENDING');
        setClusterFilterDraft('');
        setNamespaceFilterDraft('');
        adoptions.clearFilters();
    };

    const decisionAction = adoptions.decision?.action;
    const decisionRecord = adoptions.decision?.record;
    const decisionIsReject = decisionAction === 'reject';

    return (
        <div data-testid="admin-pending-adoptions-page" className="admin-pending-adoptions-page">
            <PageHeader
                title={t('adoptions.title', 'Pending Adoptions')}
                subtitle={t(
                    'adoptions.subtitle',
                    'Review KubeVirt resources discovered in live clusters and adopt them into Shepherd inventory.',
                )}
                actions={(
                    <Button
                        icon={<ReloadOutlined />}
                        data-testid="adoptions-refresh-btn"
                        onClick={() => adoptions.refetch()}
                    >
                        {t('common:button.refresh')}
                    </Button>
                )}
            />

            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('adoptions.summary.visible_title', 'Visible candidates')}
                    value={adoptionSummary.visible}
                    description={t('adoptions.summary.visible_description', 'Candidates matching the current status and search filters.')}
                    visual={<SystemsOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('adoptions.summary.pending_title', 'Pending')}
                    value={adoptionSummary.pending}
                    description={t('adoptions.summary.pending_description', 'Live resources waiting for an administrator decision.')}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#D66A1F"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('adoptions.summary.adopted_title', 'Adopted')}
                    value={adoptionSummary.adopted}
                    description={t('adoptions.summary.adopted_description', 'Resources already linked back into Shepherd inventory.')}
                    visual={<HealthOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('adoptions.summary.rejected_title', 'Rejected')}
                    value={adoptionSummary.rejected}
                    description={t('adoptions.summary.rejected_description', 'Resources deliberately left outside Shepherd management.')}
                    visual={<NotificationInboxGlyph className="summary-metric-card__art" />}
                    accentColor="#B42318"
                    surfaceColor="#FFF1F0"
                />
            </div>

            <PageSurface flush={true}>
                <PageSearchToolbar
                    searchValue={adoptions.search}
                    searchDraftValue={quickSearchDraft}
                    onSearchDraftChange={setQuickSearchDraft}
                    onSearchChange={(value) => {
                        setQuickSearchDraft(value);
                        adoptions.changeSearch(value);
                    }}
                    searchPlaceholder={t('adoptions.search_placeholder', 'Search resource, namespace, cluster, or discovery actor')}
                    searchHelp={t('adoptions.search_help', 'Press Enter or click Search. Use advanced search for exact cluster and namespace filters.')}
                    advancedSearch={{
                        open: filtersOpen,
                        onToggle: () => setFiltersOpen((open) => !open),
                        openLabel: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                        closeLabel: t('common:search.hide_advanced', { defaultValue: 'Hide advanced search' }),
                        title: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                        content: (
                            <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                <Text type="secondary">
                                    {t('adoptions.advanced_search_help', 'Choose exact workflow and placement filters for discovered KubeVirt resources.')}
                                </Text>
                                <Space wrap>
                                    <Select
                                        placeholder={t('common:table.status')}
                                        allowClear
                                        showSearch
                                        filterOption={filterOptionByLabel}
                                        optionFilterProp="label"
                                        style={{ width: 180 }}
                                        data-testid="adoptions-status-filter"
                                        value={statusFilterDraft || undefined}
                                        onChange={(value) =>
                                            setStatusFilterDraft((value as PendingAdoptionStatus | undefined) ?? '')
                                        }
                                        options={statusOptions}
                                    />
                                    <Input
                                        placeholder={t('adoptions.filter.cluster', 'Cluster ID')}
                                        style={{ width: 220 }}
                                        data-testid="adoptions-cluster-filter"
                                        value={clusterFilterDraft}
                                        onChange={(event) => setClusterFilterDraft(event.target.value)}
                                    />
                                    <Input
                                        placeholder={t('adoptions.filter.namespace', 'Namespace')}
                                        style={{ width: 180 }}
                                        data-testid="adoptions-namespace-filter"
                                        value={namespaceFilterDraft}
                                        onChange={(event) => setNamespaceFilterDraft(event.target.value)}
                                    />
                                    <Button
                                        type="primary"
                                        data-testid="adoptions-advanced-search-submit"
                                        onClick={applyAdvancedFilters}
                                    >
                                        {t('common:button.search')}
                                    </Button>
                                </Space>
                            </Space>
                        ),
                    }}
                    hasActiveFilters={Boolean(
                        adoptions.search.trim() ||
                            adoptions.statusFilter !== 'PENDING' ||
                            adoptions.clusterFilter.trim() ||
                            adoptions.namespaceFilter.trim(),
                    )}
                    onClear={clearFilters}
                    clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                />

                <Table<PendingAdoption>
                    style={{ marginTop: 16 }}
                    columns={columns}
                    dataSource={adoptionItems}
                    rowKey="id"
                    loading={adoptions.isLoading}
                    pagination={{
                        current: adoptions.page,
                        total: adoptions.data?.pagination?.total ?? 0,
                        pageSize: PENDING_ADOPTION_PAGE_SIZE,
                        onChange: adoptions.setPage,
                        showTotal: (total) => t('common:table.total', { total }),
                    }}
                    size="middle"
                    scroll={{ x: 1280 }}
                    locale={{
                        emptyText: (
                            <ActionEmptyState
                                compact={true}
                                title={t('adoptions.empty.title', 'No adoption candidates found')}
                                description={t('adoptions.empty.description', 'Discovery has not found unmanaged KubeVirt VMs matching the current filters.')}
                                visual={<QueueReviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                            />
                        ),
                    }}
                />
            </PageSurface>

            <Modal
                title={decisionIsReject
                    ? t('adoptions.reject.title', 'Reject adoption candidate')
                    : t('adoptions.adopt.title', 'Adopt live resource')}
                open={Boolean(adoptions.decision)}
                okText={decisionIsReject
                    ? t('common:button.reject')
                    : t('adoptions.action.adopt', 'Adopt')}
                okButtonProps={{ danger: decisionIsReject }}
                confirmLoading={adoptions.decisionPending}
                onOk={adoptions.submitDecision}
                onCancel={adoptions.closeDecision}
                data-testid="adoption-decision-modal"
            >
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                    <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                        {decisionIsReject
                            ? t('adoptions.reject.description', {
                                defaultValue: 'Reject {{resource}} and keep it outside Shepherd inventory.',
                                resource: decisionRecord?.resource_name ?? '',
                            })
                            : t('adoptions.adopt.description', {
                                defaultValue: 'Adopt {{resource}} into Shepherd VM inventory using its live KubeVirt state.',
                                resource: decisionRecord?.resource_name ?? '',
                            })}
                    </Paragraph>
                    <Input.TextArea
                        value={adoptions.decision?.reason ?? ''}
                        onChange={(event) => adoptions.setDecisionReason(event.target.value)}
                        maxLength={512}
                        showCount
                        autoSize={{ minRows: 3, maxRows: 6 }}
                        placeholder={t('adoptions.decision.reason_placeholder', 'Optional audit reason')}
                        data-testid="adoption-decision-reason"
                    />
                </Space>
            </Modal>
        </div>
    );
}
