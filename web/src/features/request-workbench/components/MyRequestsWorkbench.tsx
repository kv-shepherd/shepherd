'use client';

import { useEffect, useMemo, useState, type ReactNode } from 'react';
import {
    AuditOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import {
    Alert,
    Badge,
    Button,
    Card,
    Descriptions,
    Drawer,
    Segmented,
    Space,
    Table,
    Tabs,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useRouter, useSearchParams } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    BatchFlowGlyph,
    DraftNotebookGlyph,
    QueueReviewGlyph,
    RequestsOverviewGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useMyRequestsController } from '../hooks/useMyRequestsController';
import {
    approvalEmptyValue,
    approvalPrimaryAlert,
    approvalSummaryMeta,
    approvalSummaryTitle,
    buildApprovalBatchDisplayItems,
    buildApprovalChangeItems,
    buildApprovalOverviewItems,
    buildApprovalScopeItems,
    formatApprovalRecordID,
} from '@/features/approval-shared/summary';
import type {
    ApprovalStatus,
    Ticket,
    HistoryStatusFilter,
    RequestWorkbenchView,
} from '../types';
import { STATUS_COLORS } from '../types';

const { Text } = Typography;
const EMPTY_VALUE = approvalEmptyValue();

const HISTORY_STATUS_OPTIONS: Array<{ value: HistoryStatusFilter; labelKey: string }> = [
    { value: 'SUCCESS', labelKey: 'status.SUCCESS' },
    { value: 'FAILED', labelKey: 'status.FAILED' },
    { value: 'REJECTED', labelKey: 'status.REJECTED' },
    { value: 'CANCELLED', labelKey: 'status.CANCELLED' },
];

function batchChildSummaryTitle(resourceName: string | undefined): string {
    return resourceName && resourceName.trim() !== '' ? resourceName : EMPTY_VALUE;
}

export function MyRequestsWorkbench() {
    const router = useRouter();
    const searchParams = useSearchParams();
    const { t } = useTranslation(['approval', 'common', 'vm']);
    const requests = useMyRequestsController({ t });
    const setupGuide = useSetupGuide();
    const requestedTab = searchParams.get('tab');
    const { changeView, view } = requests;
    const [detailTicket, setDetailTicket] = useState<Ticket | null>(null);

    useEffect(() => {
        if (
            requestedTab &&
            ['drafts', 'in_progress', 'history', 'batch_jobs'].includes(requestedTab) &&
            requestedTab !== view
        ) {
            changeView(requestedTab as RequestWorkbenchView);
        }
    }, [changeView, requestedTab, view]);

    const openRequestDetails = (ticket: Ticket) => {
        setDetailTicket(ticket);
    };

    const closeRequestDetails = () => {
        setDetailTicket(null);
    };

    const openRequestContext = (ticket: Ticket) => {
        const params = new URLSearchParams({ request: 'create' });
        if (ticket.request_prefill?.system_id) {
            params.set('system_id', ticket.request_prefill.system_id);
        }
        if (ticket.request_prefill?.service_id) {
            params.set('service_id', ticket.request_prefill.service_id);
        }
        router.push(`/vms?${params.toString()}`);
    };

    const reuseRequest = (ticket: Ticket) => {
        if (requests.prepareHistoryReuse(ticket)) {
            closeRequestDetails();
            router.push('/vms?request=create&draft=resume');
        }
    };

    const approveAlert = useMemo(() => (
        detailTicket ? approvalPrimaryAlert(detailTicket, t) : null
    ), [detailTicket, t]);

    const requestDetailItems = useMemo(() => {
        if (!detailTicket) {
            return [];
        }

        return [
            { key: 'ticket_id', label: t('ticket_id'), children: detailTicket.id },
            ...buildApprovalOverviewItems(detailTicket, t),
            { key: 'status', label: t('common:table.status'), children: t(`status.${detailTicket.status}`) },
            { key: 'created', label: t('common:table.created_at'), children: <LocalDateTimeText value={detailTicket.created_at} /> },
            {
                key: 'updated',
                label: t('workbench.details.updated_at'),
                children: detailTicket.updated_at ? <LocalDateTimeText value={detailTicket.updated_at} /> : EMPTY_VALUE,
            },
        ];
    }, [detailTicket, t]);

    const requestScopeItems = useMemo(() => (
        detailTicket ? buildApprovalScopeItems(detailTicket, t) : []
    ), [detailTicket, t]);

    const requestChangeItems = useMemo(() => (
        detailTicket ? buildApprovalChangeItems(detailTicket, t) : []
    ), [detailTicket, t]);

    const requestBatchItems = useMemo(() => (
        detailTicket ? buildApprovalBatchDisplayItems(detailTicket, t) : []
    ), [detailTicket, t]);

    const requestContextItems = useMemo(() => {
        if (!detailTicket?.request_prefill) {
            return [];
        }

        return [
            { key: 'system', label: t('workbench.details.system'), children: detailTicket.request_prefill.system_id || EMPTY_VALUE },
            { key: 'service', label: t('workbench.details.service'), children: detailTicket.request_prefill.service_id || EMPTY_VALUE },
            { key: 'template', label: t('workbench.drafts.template'), children: detailTicket.request_prefill.template_id || EMPTY_VALUE },
            { key: 'size', label: t('workbench.drafts.size'), children: detailTicket.request_prefill.instance_size_id || EMPTY_VALUE },
            { key: 'namespace', label: t('workbench.drafts.namespace'), children: detailTicket.request_prefill.namespace || EMPTY_VALUE },
            { key: 'batch_count', label: t('workbench.drafts.batch_count'), children: detailTicket.request_prefill.batch_count },
        ];
    }, [detailTicket, t]);

    const requestListTotal = requests.data?.pagination?.total ?? 0;
    const batchStatusColor =
        requests.batchStatus?.status === 'COMPLETED'
            ? 'green'
            : requests.batchStatus?.status === 'FAILED'
                ? 'red'
                : 'blue';
    const formatBatchStatus = (status?: string) => {
        if (!status) {
            return EMPTY_VALUE;
        }
        const labelKey = `vm:batch.status_value.${status}`;
        const label = t(labelKey);
        return label === labelKey ? status : label;
    };
    const formatBatchOperation = (operation?: string) => {
        if (!operation) {
            return EMPTY_VALUE;
        }
        const labelKey = `vm:batch.operation.${operation}`;
        const label = t(labelKey);
        return label === labelKey ? operation : label;
    };

    const columns: ColumnsType<Ticket> = [
        {
            title: t('request_summary'),
            key: 'request_summary',
            width: 280,
            render: (_, record) => {
                const summaryMeta = approvalSummaryMeta(record, t);
                return (
                    <Space direction="vertical" size={0}>
                        <Space size={8}>
                            <AuditOutlined style={{ color: '#d4380d' }} />
                            <Text strong>{approvalSummaryTitle(record, t)}</Text>
                        </Space>
                        {summaryMeta.length > 0 && (
                            <Text type="secondary" style={{ fontSize: 12 }}>
                                {summaryMeta.join(' · ')}
                            </Text>
                        )}
                        <Text copyable={{ text: record.id }} type="secondary" style={{ fontSize: 12 }}>
                            {t('ticket_id')}: {formatApprovalRecordID(record.id)}
                        </Text>
                    </Space>
                );
            },
        },
        {
            title: t('operation_type'),
            dataIndex: 'operation_type',
            key: 'operation_type',
            width: 120,
            render: (opType: string) => (
                <Tag color="purple">
                    {opType ? t(`op_type.${opType}`) : EMPTY_VALUE}
                </Tag>
            ),
        },
        {
            title: t('common:table.status'),
            dataIndex: 'status',
            key: 'status',
            width: 120,
            render: (status: ApprovalStatus) => (
                <Badge
                    status={status === 'PENDING' ? 'processing' : status === 'APPROVED' ? 'success' : 'error'}
                    text={<Tag color={STATUS_COLORS[status]}>{t(`status.${status}`)}</Tag>}
                />
            ),
        },
        {
            title: t('reason'),
            dataIndex: 'reason',
            key: 'reason',
            ellipsis: true,
            render: (reason: string) => <Text type="secondary">{reason || EMPTY_VALUE}</Text>,
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 180,
            render: (date: string) => (
                <Text type="secondary">
                    <LocalDateTimeText value={date} />
                </Text>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 260,
            render: (_, record) => {
                const showReuseAction = (
                    requests.view === 'history' &&
                    record.operation_type === 'CREATE' &&
                    record.request_prefill
                );

                if (record.status === 'PENDING') {
                    return (
                        <Space wrap size="small">
                            <Button
                                size="small"
                                data-testid={`approval-action-details-${record.id}`}
                                onClick={() => openRequestDetails(record)}
                            >
                                {t('workbench.actions.details')}
                            </Button>
                            <Button
                                size="small"
                                danger
                                data-testid={`approval-action-cancel-${record.id}`}
                                loading={requests.cancelMutation.isPending}
                                onClick={() => requests.cancelMutation.mutate(record.id)}
                            >
                                {t('cancel')}
                            </Button>
                        </Space>
                    );
                }

                if (showReuseAction) {
                    return (
                        <Space wrap size="small">
                            <Button
                                size="small"
                                data-testid={`approval-action-details-${record.id}`}
                                onClick={() => openRequestDetails(record)}
                            >
                                {t('workbench.actions.details')}
                            </Button>
                            <Button
                                size="small"
                                data-testid={`approval-action-reuse-${record.id}`}
                                onClick={() => reuseRequest(record)}
                            >
                                {t('workbench.history.reuse')}
                            </Button>
                        </Space>
                    );
                }

                return (
                    <Button
                        size="small"
                        data-testid={`approval-action-details-${record.id}`}
                        onClick={() => openRequestDetails(record)}
                    >
                        {t('workbench.actions.details')}
                    </Button>
                );
            },
        },
    ];

    const renderRequestTable = () => (
        <Table<Ticket>
            columns={columns}
            dataSource={requests.data?.items ?? []}
            rowKey="id"
            loading={requests.isLoading}
            pagination={{
                current: requests.page,
                pageSize: requests.pageSize,
                total: requests.data?.pagination?.total ?? 0,
                showTotal: (total) => t('common:table.total', { total }),
                onChange: (page, pageSize) => {
                    requests.setPage(page);
                    requests.setPageSize(pageSize);
                },
            }}
            size="middle"
        />
    );

    const renderEmptyState = (
        titleKey: string,
        descriptionKey: string,
        buttonKey: string,
        options?: {
            showSetupGuide?: boolean;
            visual?: ReactNode;
        },
    ) => (
        options?.showSetupGuide && !setupGuide.vmRequestReady ? (
            <div style={{ padding: 24 }}>
                <SetupGuideCard variant="vm" surface={false} />
            </div>
        ) : (
            <div style={{ padding: 32 }}>
                <ActionEmptyState
                    title={t(titleKey)}
                    description={t(descriptionKey)}
                    visual={options?.visual}
                    actions={(
                    <Button type="primary" onClick={() => router.push('/vms?request=create')}>
                        {t(buttonKey)}
                    </Button>
                    )}
                />
            </div>
        )
    );

    const renderSummaryCards = () => (
        <div className="summary-card-grid">
            <SummaryMetricCard
                title={t('workbench.summary.draft_title')}
                value={requests.savedVmDraft ? t('workbench.summary.draft_ready') : t('workbench.summary.draft_empty')}
                description={(
                    <Space direction="vertical" size={2}>
                        <span>{t('workbench.summary.draft_description')}</span>
                        {requests.savedVmDraft?.updatedAt ? (
                            <span>
                                {t('workbench.drafts.updated_at')}: <LocalDateTimeText value={requests.savedVmDraft.updatedAt} />
                            </span>
                        ) : null}
                    </Space>
                )}
                action={(
                    <Button
                        size="small"
                        type={requests.savedVmDraft ? 'primary' : 'default'}
                        onClick={() => router.push(requests.savedVmDraft ? '/vms?request=create&draft=resume' : '/vms?request=create')}
                    >
                        {t('workbench.summary.draft_cta')}
                    </Button>
                )}
                visual={<DraftNotebookGlyph className="summary-metric-card__art" />}
                accentColor="#1D5BFF"
                surfaceColor="#E6F4FF"
            />

            <SummaryMetricCard
                title={t('workbench.summary.in_progress_title')}
                value={requests.view === 'in_progress' ? requestListTotal : EMPTY_VALUE}
                description={
                    requests.view === 'in_progress'
                        ? t('workbench.summary.pending_description', { count: requestListTotal })
                        : t('workbench.summary.load_on_open')
                }
                action={(
                    <Button size="small" onClick={() => requests.changeView('in_progress')}>
                        {t('workbench.summary.in_progress_cta')}
                    </Button>
                )}
                visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                accentColor="#D97706"
                surfaceColor="#FFF4E5"
            />

            <SummaryMetricCard
                title={t('workbench.summary.history_title')}
                value={requests.view === 'history' ? requestListTotal : EMPTY_VALUE}
                description={
                    requests.view === 'history'
                        ? t('workbench.summary.history_description', { count: requestListTotal })
                        : t('workbench.summary.load_on_open')
                }
                action={(
                    <Button size="small" onClick={() => requests.changeView('history')}>
                        {t('workbench.summary.history_cta')}
                    </Button>
                )}
                visual={<RequestsOverviewGlyph className="summary-metric-card__art" />}
                accentColor="#7C3AED"
                surfaceColor="#F5EDFF"
            />

            <SummaryMetricCard
                title={t('workbench.summary.batch_title')}
                value={(
                    <Space wrap size={8}>
                        <span>{requests.activeBatchID || t('workbench.summary.batch_inactive')}</span>
                        {requests.batchStatus?.status ? (
                            <Tag color={batchStatusColor}>{formatBatchStatus(requests.batchStatus.status)}</Tag>
                        ) : null}
                    </Space>
                )}
                description={t('workbench.summary.batch_description')}
                action={(
                    <Button size="small" onClick={() => requests.changeView('batch_jobs')}>
                        {t('workbench.summary.batch_cta')}
                    </Button>
                )}
                visual={<BatchFlowGlyph className="summary-metric-card__art" />}
                accentColor="#D66A1F"
                surfaceColor="#FFF1E8"
            />
        </div>
    );

    const renderDrafts = () => {
        if (!requests.savedVmDraft) {
            return renderEmptyState(
                'workbench.drafts.empty_title',
                'workbench.drafts.empty_description',
                'workbench.open_vms',
                { showSetupGuide: true },
            );
        }

        return (
            <Card variant="borderless" styles={{ body: { padding: 0 } }}>
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                    <Space direction="vertical" size={4}>
                        <Text strong>{t('workbench.drafts.saved_title')}</Text>
                        <Text type="secondary">{t('workbench.drafts.saved_description')}</Text>
                    </Space>
                    <Descriptions bordered size="small" column={2}>
                        <Descriptions.Item label={t('workbench.drafts.service')}>
                            {requests.savedVmDraft.serviceLabel || requests.savedVmDraft.serviceId || EMPTY_VALUE}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.template')}>
                            {requests.savedVmDraft.templateLabel || requests.savedVmDraft.templateId || EMPTY_VALUE}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.size')}>
                            {requests.savedVmDraft.instanceSizeLabel || requests.savedVmDraft.instanceSizeId || EMPTY_VALUE}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.namespace')}>
                            {requests.savedVmDraft.namespace || EMPTY_VALUE}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.batch_count')}>
                            {requests.savedVmDraft.batchCount}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.updated_at')}>
                            <LocalDateTimeText value={requests.savedVmDraft.updatedAt} />
                        </Descriptions.Item>
                    </Descriptions>
                    <Space wrap>
                        <Button type="primary" onClick={() => router.push('/vms?request=create&draft=resume')}>
                            {t('workbench.drafts.resume')}
                        </Button>
                        <Button onClick={requests.discardSavedVmDraft}>
                            {t('workbench.drafts.discard')}
                        </Button>
                    </Space>
                </Space>
            </Card>
        );
    };

    const renderBatchJobs = () => {
        if (!requests.activeBatchID) {
            return renderEmptyState(
                'workbench.batch_jobs.empty_title',
                'workbench.batch_jobs.empty_description',
                'workbench.open_vms',
                { showSetupGuide: true, visual: <BatchFlowGlyph className="action-empty-state__art" /> },
            );
        }

        return (
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Card
                    variant="borderless"
                    title={t('workbench.batch_jobs.current_title')}
                    extra={(
                        <Space>
                            <Button
                                icon={<ReloadOutlined />}
                                onClick={requests.refreshBatch}
                                loading={requests.batchLoading}
                            >
                                {t('common:button.refresh')}
                            </Button>
                            <Button onClick={requests.clearBatchTracking}>
                                {t('vm:batch.clear')}
                            </Button>
                        </Space>
                    )}
                >
                    <Space direction="vertical" size={16} style={{ width: '100%' }}>
                        <Alert
                            type={requests.batchStatus?.status === 'FAILED' ? 'warning' : 'info'}
                            showIcon
                            message={t('workbench.batch_jobs.status_summary', {
                                status: formatBatchStatus(requests.batchStatus?.status),
                                success: requests.batchStatus?.success_count ?? 0,
                                failed: requests.batchStatus?.failed_count ?? 0,
                                pending: requests.batchStatus?.pending_count ?? 0,
                            })}
                        />
                        <Descriptions bordered size="small" column={2}>
                            <Descriptions.Item label={t('workbench.batch_jobs.batch_id')}>
                                {requests.activeBatchID}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.status')}>
                                <Tag color={
                                    requests.batchStatus?.status === 'COMPLETED'
                                        ? 'green'
                                        : requests.batchStatus?.status === 'FAILED'
                                            ? 'red'
                                            : 'blue'
                                }>
                                    {formatBatchStatus(requests.batchStatus?.status)}
                                </Tag>
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.operation')}>
                                {formatBatchOperation(requests.batchStatus?.operation)}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.child_count')}>
                                {requests.batchStatus?.child_count ?? 0}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.success_count')}>
                                {requests.batchStatus?.success_count ?? 0}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.failed_count')}>
                                {requests.batchStatus?.failed_count ?? 0}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.pending_count')}>
                                {requests.batchStatus?.pending_count ?? 0}
                            </Descriptions.Item>
                        </Descriptions>
                        <Space wrap>
                            <Button
                                onClick={requests.retryBatch}
                                disabled={!requests.batchCanRetry}
                                loading={requests.batchActionPending}
                            >
                                {t('vm:batch.retry_failed')}
                            </Button>
                            <Button
                                danger
                                onClick={requests.cancelBatch}
                                disabled={!requests.batchCanCancel}
                                loading={requests.batchActionPending}
                            >
                                {t('vm:batch.cancel_pending')}
                            </Button>
                            <Button onClick={() => router.push('/vms')}>
                                {t('workbench.open_vms')}
                            </Button>
                        </Space>
                    </Space>
                </Card>

                <Card variant="borderless" title={t('workbench.batch_jobs.child_title')}>
                    <Table
                        rowKey="ticket_id"
                        loading={requests.batchLoading}
                        dataSource={requests.batchStatus?.children ?? []}
                        pagination={false}
                        columns={[
                            {
                                title: t('vm:batch.child.resource'),
                                key: 'resource_summary',
                                render: (_, record) => (
                                    <Space direction="vertical" size={0}>
                                        <Text strong>{batchChildSummaryTitle(record.resource_name)}</Text>
                                        <Text copyable={{ text: record.ticket_id }} type="secondary" style={{ fontSize: 12 }}>
                                            {t('vm:batch.child.ticket')}: {formatApprovalRecordID(record.ticket_id)}
                                        </Text>
                                    </Space>
                                ),
                            },
                            {
                                title: t('vm:batch.child.status'),
                                dataIndex: 'status',
                                key: 'status',
                                render: (status: string) => <Tag>{formatBatchStatus(status)}</Tag>,
                            },
                            { title: t('vm:batch.child.attempt'), dataIndex: 'attempt_count', key: 'attempt_count' },
                            { title: t('vm:batch.child.error'), dataIndex: 'last_error', key: 'last_error' },
                        ]}
                    />
                </Card>
            </Space>
        );
    };

    const tabItems: Array<{ key: RequestWorkbenchView; label: string; children: ReactNode }> = [
        {
            key: 'drafts',
            label: t('workbench.tab.drafts'),
            children: renderDrafts(),
        },
        {
            key: 'in_progress',
            label: t('workbench.tab.in_progress'),
            children: (
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                    <Text type="secondary">{t('workbench.in_progress.description')}</Text>
                    {(requests.data?.items?.length ?? 0) === 0 && !requests.isLoading
                        ? renderEmptyState(
                            'workbench.in_progress.empty_title',
                            'workbench.in_progress.empty_description',
                            'workbench.open_vms',
                            { showSetupGuide: true, visual: <QueueReviewGlyph className="action-empty-state__art" /> },
                        )
                        : renderRequestTable()}
                </Space>
            ),
        },
        {
            key: 'history',
            label: t('workbench.tab.history'),
            children: (
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <Space direction="vertical" size={2}>
                            <Text type="secondary">{t('workbench.history.description')}</Text>
                            <Text type="secondary">{t('workbench.history.filter_label')}</Text>
                        </Space>
                        <Segmented
                            data-testid="approvals-status-filter"
                            value={requests.historyStatus}
                            onChange={(value) => requests.changeHistoryStatus(value as HistoryStatusFilter)}
                            options={HISTORY_STATUS_OPTIONS.map((option) => ({
                                label: t(option.labelKey),
                                value: option.value,
                            }))}
                        />
                    </div>
                    {(requests.data?.items?.length ?? 0) === 0 && !requests.isLoading
                        ? renderEmptyState(
                            'workbench.history.empty_title',
                            'workbench.history.empty_description',
                            'workbench.open_vms',
                            { showSetupGuide: true, visual: <RequestsOverviewGlyph className="action-empty-state__art" /> },
                        )
                        : renderRequestTable()}
                </Space>
            ),
        },
        {
            key: 'batch_jobs',
            label: t('workbench.tab.batch_jobs'),
            children: renderBatchJobs(),
        },
    ];

    return (
        <div data-testid="approvals-page">
            {requests.messageContextHolder}
            <PageHeader
                title={t('my_approvals_title')}
                subtitle={t('my_approvals_subtitle')}
                actions={(
                    <Space>
                    {(requests.view === 'in_progress' || requests.view === 'history') && (
                        <Button icon={<ReloadOutlined />} onClick={() => requests.refetch()}>
                            {t('common:button.refresh')}
                        </Button>
                    )}
                    </Space>
                )}
            />

            {renderSummaryCards()}

            <PageSurface>
                <Tabs
                    activeKey={requests.view}
                    onChange={(key) => requests.changeView(key as RequestWorkbenchView)}
                    items={tabItems}
                />
            </PageSurface>

            <Drawer
                title={(
                    <Space wrap>
                        <span>{t('workbench.details.title')}</span>
                        {detailTicket ? (
                            <Tag color={STATUS_COLORS[detailTicket.status]}>
                                {t(`status.${detailTicket.status}`)}
                            </Tag>
                        ) : null}
                        {detailTicket?.operation_type ? (
                            <Tag color="purple">{t(`op_type.${detailTicket.operation_type}`)}</Tag>
                        ) : null}
                    </Space>
                )}
                open={detailTicket !== null}
                onClose={closeRequestDetails}
                width={720}
                forceRender={true}
                footer={(
                    <Space wrap>
                        <Button onClick={closeRequestDetails}>
                            {t('common:button.close')}
                        </Button>
                        {detailTicket?.request_prefill && (
                            <Button onClick={() => openRequestContext(detailTicket)}>
                                {t('workbench.details.open_request_context')}
                            </Button>
                        )}
                        {detailTicket?.status === 'PENDING' && (
                            <Button
                                danger
                                loading={requests.cancelMutation.isPending}
                                onClick={() => {
                                    requests.cancelMutation.mutate(detailTicket.id);
                                    closeRequestDetails();
                                }}
                            >
                                {t('cancel')}
                            </Button>
                        )}
                        {requests.view === 'history' &&
                            detailTicket?.operation_type === 'CREATE' &&
                            detailTicket.request_prefill && (
                            <Button
                                type="primary"
                                onClick={() => reuseRequest(detailTicket)}
                            >
                                {t('workbench.history.reuse')}
                            </Button>
                        )}
                    </Space>
                )}
            >
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                    {approveAlert ? (
                        <Alert
                            type={approveAlert.type}
                            showIcon
                            message={approveAlert.message}
                            description={approveAlert.description}
                        />
                    ) : null}
                    {detailTicket ? (
                        <Alert
                            type={detailTicket.status === 'PENDING' ? 'info' : 'success'}
                            showIcon
                            message={detailTicket.status === 'PENDING'
                                ? t('workbench.details.pending_hint')
                                : t('workbench.details.completed_hint')}
                        />
                    ) : null}
                    <Card variant="borderless" title={t('workbench.details.summary_title')}>
                        <Descriptions
                            bordered
                            size="small"
                            column={2}
                            items={requestDetailItems}
                        />
                    </Card>
                    {requestScopeItems.length > 0 && (
                        <Card variant="borderless" title={t('workbench.details.resource_title')}>
                            <Descriptions
                                bordered
                                size="small"
                                column={2}
                                items={requestScopeItems}
                            />
                        </Card>
                    )}
                    {requestChangeItems.length > 0 && (
                        <Card variant="borderless" title={t('workbench.details.change_title')}>
                            <Descriptions
                                bordered
                                size="small"
                                column={2}
                                items={requestChangeItems}
                            />
                        </Card>
                    )}
                    {requestBatchItems.length > 0 && (
                        <Card variant="borderless" title={t('workbench.details.affected_items_title')}>
                            <Table
                                rowKey="key"
                                size="small"
                                pagination={false}
                                dataSource={requestBatchItems}
                                columns={[
                                    { title: t('summary.item'), dataIndex: 'title', key: 'title' },
                                    { title: t('summary.scope'), dataIndex: 'scope', key: 'scope', render: (value: string | undefined) => value || EMPTY_VALUE },
                                    { title: t('summary.cluster'), dataIndex: 'cluster', key: 'cluster', render: (value: string | undefined) => value || EMPTY_VALUE },
                                    { title: t('summary.request_vm_status'), dataIndex: 'requestStatus', key: 'requestStatus', render: (value: string | undefined) => value || EMPTY_VALUE },
                                    {
                                        title: t('summary.latest_vm_status'),
                                        dataIndex: 'latestStatus',
                                        key: 'latestStatus',
                                        render: (value: string | undefined, record) => (
                                            <Space direction="vertical" size={0}>
                                                <span>{value || EMPTY_VALUE}</span>
                                                {record.statusChanged ? (
                                                    <Text type="warning" style={{ fontSize: 12 }}>
                                                        {t('summary.status_changed')}
                                                    </Text>
                                                ) : null}
                                            </Space>
                                        ),
                                    },
                                    { title: t('summary.current_resources'), dataIndex: 'currentShape', key: 'currentShape', render: (value: string | undefined) => value || EMPTY_VALUE },
                                    { title: t('summary.target_resources'), dataIndex: 'targetShape', key: 'targetShape', render: (value: string | undefined) => value || EMPTY_VALUE },
                                    { title: t('summary.power_action'), dataIndex: 'action', key: 'action', render: (value: string | undefined) => value || EMPTY_VALUE },
                                ]}
                            />
                        </Card>
                    )}
                    <Card variant="borderless" title={t('workbench.details.context_title')}>
                        {detailTicket?.request_prefill ? (
                            <Descriptions
                                bordered
                                size="small"
                                column={2}
                                items={requestContextItems}
                            />
                        ) : (
                            <ActionEmptyState
                                compact={true}
                                title={t('workbench.details.context_title')}
                                description={t('workbench.details.no_context')}
                                visual={<RequestsOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                            />
                        )}
                    </Card>
                </Space>
            </Drawer>
        </div>
    );
}
