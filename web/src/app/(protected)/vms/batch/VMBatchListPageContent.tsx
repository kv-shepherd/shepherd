'use client';

/**
 * /vms/batch — Batch VM Operation Status list page.
 * master-flow.md §5.E: Batch Operations.
 *
 * API contracts:
 *   GET /vms/batch                  → VMBatchList
 *
 * E2E data-testid requirements:
 *   vm-batch-page
 *   batch-action-detail-{id}
 */
import { Alert, Button, Select, Space, Table, Tag, Typography } from 'antd';
import type { TablePaginationConfig } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    DownloadOutlined,
    EyeOutlined,
    RedoOutlined,
    ReloadOutlined,
    StopOutlined,
    ThunderboltOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import type { TFunction } from 'i18next';
import { useRouter } from 'next/navigation';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useApiMutation } from '@/hooks/useApiQuery';
import { useApiGet } from '@/lib/api/useApiGet';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useMessage } from '@/lib/hooks/useMessage';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import {
    canExportBatchResult,
    downloadBatchResultExport,
    type VMBatchStatus,
} from '@/features/vm-management/batchExport';
import {
    canManageBatchOperation,
    createBatchActionFeedback,
    extractRestartReconciliationNotice,
    isBatchConflictError,
    isRetryableBatchChild,
    type BatchActionFeedback,
    type RestartReconciliationNotice,
} from '@/features/vm-management/batchActions';
import { useBatchActionCooldown } from '@/features/vm-management/hooks/useBatchActionCooldown';
import { useAuthStore } from '@/stores/auth';
import type { components } from '@/types/api.gen';

const { Text } = Typography;

const filterOptionByLabel = (input: string, option?: { label?: unknown }) => {
    const label = typeof option?.label === 'string' ? option.label : '';
    return label.toLowerCase().includes(input.trim().toLowerCase());
};

type BatchJobSummary = components['schemas']['VMBatchJobSummary'];
type BatchJobList = components['schemas']['VMBatchList'];
type VMBatchActionResponse = components['schemas']['VMBatchActionResponse'];
type BatchRetryMutationInput = {
    batchId: string;
    targetTicketIDs: string[];
};

const STATUS_COLORS: Record<string, string> = {
    COMPLETED: 'green',
    FAILED: 'red',
    IN_PROGRESS: 'blue',
    PENDING_APPROVAL: 'orange',
    PARTIAL_SUCCESS: 'warning',
    CANCELLED: 'default',
};

function batchStatusLabel(status: string, t: TFunction) {
    const labelKey = `batch.status_value.${status}`;
    const label = t(labelKey);
    return label === labelKey ? status : label;
}

function batchOperationLabel(operation: string, t: TFunction) {
    const labelKey = `batch.operation.${operation}`;
    const label = t(labelKey);
    return label === labelKey ? operation : label;
}

function formatRecordID(id: string): string {
    if (id.length <= 14) {
        return id;
    }
    return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

export default function VMBatchListPage() {
    const { t } = useTranslation(['vm', 'common']);
    const router = useRouter();
    const { messageApi, messageContextHolder } = useMessage();
    const user = useAuthStore((state) => state.user);
    const cooldown = useBatchActionCooldown();
    const [quickSearchDraft, setQuickSearchDraft] = useState('');
    const [search, setSearch] = useState('');
    const [operationDraft, setOperationDraft] = useState('');
    const [statusDraft, setStatusDraft] = useState('');
    const [operationFilter, setOperationFilter] = useState('');
    const [statusFilter, setStatusFilter] = useState('');
    const [advancedSearchOpen, setAdvancedSearchOpen] = useState(false);
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [actionFeedback, setActionFeedback] = useState<BatchActionFeedback | null>(null);
    const [restartReconciliationNotice, setRestartReconciliationNotice] = useState<RestartReconciliationNotice | null>(
        null,
    );

    const { data, isLoading, refetch } = useApiGet<BatchJobList>(
        ['vm-batch-list', page, pageSize],
        () =>
            api.GET('/vms/batch', {
                params: { query: { page, per_page: pageSize } },
            }) as Promise<{
                data?: BatchJobList;
                error?: unknown;
                response?: Response;
            }>,
    );

    const retryMutation = useApiMutation<BatchRetryMutationInput, VMBatchActionResponse>(
        async (submission) => {
            // The list projection intentionally aggregates FAILED and REJECTED
            // children into failed_count. Load the authoritative child view
            // before retrying so an approval rejection is never presented as
            // an execution failure that can be retried.
            const detail = await api.GET('/vms/batch/{batch_id}', {
                params: { path: { batch_id: submission.batchId } },
            });
            if (detail.error) {
                return detail;
            }
            if (!detail.data) {
                return {
                    error: {
                        code: 'EMPTY_RESPONSE',
                        message: 'No batch detail returned',
                    },
                };
            }
            const retryableChildren = (detail.data.children ?? []).filter(isRetryableBatchChild);
            if (retryableChildren.length === 0) {
                return {
                    error: {
                        code: 'BATCH_NOTHING_TO_RETRY',
                        message: 'No failed batch children are eligible for retry',
                    },
                };
            }
            submission.targetTicketIDs = retryableChildren.map((child) => child.ticket_id);
            return api.POST('/vms/batch/{batch_id}/retry', {
                params: { path: { batch_id: submission.batchId } },
            });
        },
        {
            onSuccess: (response, submission) => {
                const feedback = createBatchActionFeedback('retry', response, submission.targetTicketIDs);
                setActionFeedback(feedback);
                void messageApi.success(
                    t('batch.retry_feedback', {
                        count: feedback.affectedCount,
                        ids: feedback.affectedTicketIDs.join(', ') || t('batch.affected_ids_none'),
                    }),
                );
                void refetch();
            },
            onError: (err) => {
                const notice = extractRestartReconciliationNotice(err);
                if (notice) {
                    setRestartReconciliationNotice(notice);
                }
                if (cooldown.capture(err)) {
                    return;
                }
                if (isBatchConflictError(err)) {
                    void refetch();
                }
                void messageApi.error(translateApiError(t, err));
            },
        },
    );

    const cancelMutation = useApiMutation<string, VMBatchActionResponse>(
        (batchId) =>
            api.POST('/vms/batch/{batch_id}/cancel', {
                params: { path: { batch_id: batchId } },
            }),
        {
            onSuccess: (response) => {
                const feedback = createBatchActionFeedback('cancel', response);
                setActionFeedback(feedback);
                void messageApi.success(
                    t('batch.cancel_feedback', {
                        count: feedback.affectedCount,
                        ids: feedback.affectedTicketIDs.join(', ') || t('batch.affected_ids_none'),
                    }),
                );
                void refetch();
            },
            onError: (err) => {
                if (cooldown.capture(err)) {
                    return;
                }
                if (isBatchConflictError(err)) {
                    void refetch();
                }
                void messageApi.error(translateApiError(t, err));
            },
        },
    );

    const [exportingBatchID, setExportingBatchID] = useState<string | null>(null);
    const exportMutation = useApiMutation<string, VMBatchStatus>(
        (batchId) =>
            api.GET('/vms/batch/{batch_id}', {
                params: { path: { batch_id: batchId } },
            }),
        {
            onSuccess: (batch) => {
                downloadBatchResultExport(batch);
                void messageApi.success(t('batch.export_started', { batch_id: batch.batch_id }));
                setExportingBatchID(null);
            },
            onError: (err) => {
                void messageApi.error(translateApiError(t, err));
                setExportingBatchID(null);
            },
        },
    );

    const canRetry = (status: string) =>
        status === 'IN_PROGRESS' || status === 'FAILED' || status === 'PARTIAL_SUCCESS';
    const canCancel = (status: string) => status === 'PENDING_APPROVAL' || status === 'IN_PROGRESS';
    const batchActionPending = retryMutation.isPending || cancelMutation.isPending;
    const handleExportResult = (batchId: string) => {
        setExportingBatchID(batchId);
        exportMutation.mutate(batchId);
    };
    const batchItems = useMemo(() => data?.items ?? [], [data?.items]);
    const operationOptions = useMemo(
        () =>
            Array.from(new Set(batchItems.map((item) => item.operation).filter(Boolean)))
                .sort((left, right) => left.localeCompare(right))
                .map((operation) => ({
                    value: operation,
                    label: batchOperationLabel(operation, t),
                })),
        [batchItems, t],
    );
    const statusOptions = useMemo(
        () =>
            Array.from(new Set(batchItems.map((item) => item.status).filter(Boolean)))
                .sort((left, right) => left.localeCompare(right))
                .map((status) => ({
                    value: status,
                    label: batchStatusLabel(status, t),
                })),
        [batchItems, t],
    );
    const filteredBatchItems = useMemo(() => {
        const normalizedSearch = search.trim().toLowerCase();
        return batchItems.filter((item) => {
            if (operationFilter !== '' && item.operation !== operationFilter) {
                return false;
            }
            if (statusFilter !== '' && item.status !== statusFilter) {
                return false;
            }
            if (normalizedSearch === '') {
                return true;
            }
            return [
                item.id,
                item.operation,
                item.status,
                batchOperationLabel(item.operation, t),
                batchStatusLabel(item.status, t),
            ].some((value) => value.toLowerCase().includes(normalizedSearch));
        });
    }, [batchItems, operationFilter, search, statusFilter, t]);

    const columns: ColumnsType<BatchJobSummary> = [
        {
            title: t('batch.summary'),
            key: 'summary',
            width: 280,
            render: (_, record) => (
                <Space direction="vertical" size={0}>
                    <Space size={8} wrap>
                        <Text strong>{batchOperationLabel(record.operation, t)}</Text>
                        <Tag color="purple">{t('batch.request_count', { count: record.child_count })}</Tag>
                    </Space>
                    <Text type="secondary" style={{ fontSize: 13 }}>
                        {t('batch.success_count')}: {record.success_count} · {t('batch.failed_count')}:{' '}
                        {record.failed_count} · {t('batch.pending_count')}: {record.pending_count}
                    </Text>
                    <Text copyable={{ text: record.id }} type="secondary" style={{ fontSize: 13 }}>
                        {t('batch.id')}: {formatRecordID(record.id)}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('batch.status'),
            dataIndex: 'status',
            key: 'status',
            width: 140,
            render: (status: string) => (
                <Tag color={STATUS_COLORS[status] ?? 'default'}>{batchStatusLabel(status, t)}</Tag>
            ),
        },
        {
            title: t('batch.child_count'),
            dataIndex: 'child_count',
            key: 'child_count',
            width: 90,
        },
        {
            title: t('batch.success_count'),
            dataIndex: 'success_count',
            key: 'success_count',
            width: 90,
        },
        {
            title: t('batch.failed_count'),
            dataIndex: 'failed_count',
            key: 'failed_count',
            width: 90,
            render: (n: number) => <Text type={n > 0 ? 'danger' : 'secondary'}>{n}</Text>,
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => <Text type="secondary">{dayjs(date).format('YYYY-MM-DD HH:mm')}</Text>,
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 160,
            render: (_, record) => (
                <Space size="small">
                    <Button
                        type="text"
                        size="small"
                        icon={<RedoOutlined />}
                        aria-label={t('batch.retry_failed')}
                        data-testid={`batch-action-retry-${record.id}`}
                        disabled={
                            !canRetry(record.status) ||
                            !canManageBatchOperation(user, record.operation) ||
                            cooldown.isActive ||
                            batchActionPending
                        }
                        loading={retryMutation.isPending}
                        onClick={() =>
                            retryMutation.mutate({
                                batchId: record.id,
                                targetTicketIDs: [],
                            })
                        }
                    />
                    <Button
                        type="text"
                        size="small"
                        danger
                        icon={<StopOutlined />}
                        aria-label={t('batch.cancel_pending')}
                        data-testid={`batch-action-cancel-${record.id}`}
                        disabled={
                            !canCancel(record.status) ||
                            !canManageBatchOperation(user, record.operation) ||
                            cooldown.isActive ||
                            batchActionPending
                        }
                        loading={cancelMutation.isPending}
                        onClick={() => cancelMutation.mutate(record.id)}
                    />
                    <Button
                        type="text"
                        size="small"
                        icon={<DownloadOutlined />}
                        aria-label={t('batch.export_result')}
                        title={t('batch.export_result')}
                        data-testid={`batch-action-export-${record.id}`}
                        disabled={
                            !canExportBatchResult(record.status) ||
                            (exportMutation.isPending && exportingBatchID !== record.id)
                        }
                        loading={exportMutation.isPending && exportingBatchID === record.id}
                        onClick={() => handleExportResult(record.id)}
                    />
                    <Button
                        type="text"
                        size="small"
                        icon={<EyeOutlined />}
                        aria-label={t('common:button.view', { defaultValue: 'View' })}
                        title={t('common:button.view', { defaultValue: 'View' })}
                        data-testid={`batch-action-detail-${record.id}`}
                        onClick={() => router.push(`/vms/batch/${record.id}`)}
                    />
                </Space>
            ),
        },
    ];

    return (
        <div data-testid="vm-batch-page">
            {messageContextHolder}
            <PageHeader
                title={
                    <Space size="small">
                        <ThunderboltOutlined style={{ color: '#fa8c16' }} />
                        <span>{t('batch.list_title')}</span>
                    </Space>
                }
                subtitle={t('batch.list_subtitle')}
                actions={
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                }
            />

            {cooldown.isActive ? (
                <Alert
                    type="warning"
                    showIcon
                    data-testid="batch-action-cooldown"
                    message={t(cooldown.contactAdmin ? 'batch.rate_limited_contact_admin' : 'batch.rate_limited_wait', {
                        seconds: cooldown.retryAfterSeconds,
                    })}
                />
            ) : null}
            {restartReconciliationNotice ? (
                <Alert
                    type="error"
                    showIcon
                    data-testid="restart-reconciliation-alert"
                    message={t('restart_reconciliation.title')}
                    description={
                        <Space direction="vertical" size={4}>
                            <Text>{t('restart_reconciliation.readonly_description')}</Text>
                            <Text copyable={{ text: restartReconciliationNotice.eventId }}>
                                {t('restart_reconciliation.event_id')}: {restartReconciliationNotice.eventId}
                            </Text>
                            <Text
                                copyable={{
                                    text: restartReconciliationNotice.reconciliationPath,
                                }}
                            >
                                {t('restart_reconciliation.path')}: {restartReconciliationNotice.reconciliationPath}
                            </Text>
                        </Space>
                    }
                />
            ) : null}
            {actionFeedback ? (
                <Alert
                    type="success"
                    showIcon
                    closable
                    data-testid="batch-action-feedback"
                    message={t(actionFeedback.action === 'retry' ? 'batch.retry_feedback' : 'batch.cancel_feedback', {
                        count: actionFeedback.affectedCount,
                        ids: actionFeedback.affectedTicketIDs.join(', ') || t('batch.affected_ids_none'),
                    })}
                    onClose={() => setActionFeedback(null)}
                />
            ) : null}

            <PageSurface flush={true}>
                <div style={{ padding: 16, paddingBottom: 0 }}>
                    <PageSearchToolbar
                        searchValue={search}
                        searchDraftValue={quickSearchDraft}
                        onSearchDraftChange={setQuickSearchDraft}
                        onSearchChange={(value) => {
                            const nextValue = value.trim();
                            setQuickSearchDraft(nextValue);
                            setSearch(nextValue);
                        }}
                        searchPlaceholder={t('batch.search_placeholder', {
                            defaultValue: 'Search batch operations on the current page or paste a batch ID',
                        })}
                        searchTestId="batch-quick-search"
                        searchHelp={t('batch.search_help', {
                            defaultValue:
                                'Search applies only to rows loaded on the current page and matches operation, status, and pasted batch IDs.',
                        })}
                        advancedSearch={{
                            open: advancedSearchOpen,
                            onToggle: () => setAdvancedSearchOpen((open) => !open),
                            openLabel: t('common:search.advanced', {
                                defaultValue: 'Advanced search',
                            }),
                            closeLabel: t('common:search.hide_advanced', {
                                defaultValue: 'Hide advanced search',
                            }),
                            title: t('batch.advanced_search_title', {
                                defaultValue: 'Filter current page',
                            }),
                            toggleTestId: 'batch-advanced-search-toggle',
                            content: (
                                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                    <Text type="secondary">
                                        {t('batch.advanced_search_help', {
                                            defaultValue:
                                                'Exact operation and status filters apply only to rows loaded on the current page.',
                                        })}
                                    </Text>
                                    <Space wrap size={[12, 12]}>
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            style={{ minWidth: 220 }}
                                            data-testid="batch-filter-operation"
                                            placeholder={t('batch.operation', {
                                                defaultValue: 'Operation',
                                            })}
                                            options={operationOptions}
                                            value={operationDraft || undefined}
                                            onChange={(value) => setOperationDraft(value ?? '')}
                                        />
                                        <Select
                                            allowClear
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            style={{ minWidth: 220 }}
                                            data-testid="batch-filter-status"
                                            placeholder={t('batch.status', {
                                                defaultValue: 'Status',
                                            })}
                                            options={statusOptions}
                                            value={statusDraft || undefined}
                                            onChange={(value) => setStatusDraft(value ?? '')}
                                        />
                                        <Button
                                            type="primary"
                                            data-testid="batch-advanced-search-submit"
                                            onClick={() => {
                                                setOperationFilter(operationDraft);
                                                setStatusFilter(statusDraft);
                                            }}
                                        >
                                            {t('common:button.search')}
                                        </Button>
                                    </Space>
                                </Space>
                            ),
                        }}
                        hasActiveFilters={search !== '' || operationFilter !== '' || statusFilter !== ''}
                        onClear={() => {
                            setQuickSearchDraft('');
                            setSearch('');
                            setOperationDraft('');
                            setStatusDraft('');
                            setOperationFilter('');
                            setStatusFilter('');
                            setAdvancedSearchOpen(false);
                        }}
                        clearLabel={t('common:button.clear_filters', {
                            defaultValue: 'Clear filters',
                        })}
                    />
                </div>
                <Table<BatchJobSummary>
                    columns={columns}
                    dataSource={filteredBatchItems}
                    rowKey="id"
                    loading={isLoading}
                    pagination={{
                        current: data?.pagination?.page ?? page,
                        pageSize: data?.pagination?.per_page ?? pageSize,
                        total: data?.pagination?.total ?? batchItems.length,
                        showSizeChanger: true,
                    }}
                    onChange={(nextPagination: TablePaginationConfig) => {
                        const nextPageSize = nextPagination.pageSize ?? pageSize;
                        if (nextPageSize !== pageSize) {
                            setPageSize(nextPageSize);
                            setPage(1);
                            return;
                        }
                        setPage(nextPagination.current ?? 1);
                    }}
                    size="middle"
                />
            </PageSurface>
        </div>
    );
}
