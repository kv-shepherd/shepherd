'use client';

/**
 * /vms/batch/[id] — Batch VM Operation Detail page.
 * master-flow.md §5.4: Batch Power Operation detail.
 *
 * API contracts:
 *   GET    /vms/batch/{batch_id}         → VMBatchStatusResponse
 *   POST   /vms/batch/{batch_id}/retry   → VMBatchActionResponse (retry failed)
 *   POST   /vms/batch/{batch_id}/cancel  → VMBatchActionResponse (cancel pending)
 *
 * E2E data-testid requirements:
 *   vm-batch-detail-page
 *   batch-status-live
 *   batch-retry-button
 *   batch-cancel-button
 */
import { Button, Descriptions, Space, Table, Tag, Typography } from 'antd';
import type { DescriptionsProps } from 'antd';
import { ArrowLeftOutlined, RedoOutlined, StopOutlined, ThunderboltOutlined } from '@ant-design/icons';
import type { TFunction } from 'i18next';
import { useParams, useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { useApiMutation } from '@/hooks/useApiQuery';
import { useApiGet } from '@/lib/api/useApiGet';
import { api } from '@/lib/api/client';
import { useMessage } from '@/lib/hooks/useMessage';
import type { components } from '@/types/api.gen';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';

const { Text } = Typography;

type BatchJobStatus = components['schemas']['VMBatchStatusResponse'];

const STATUS_COLORS: Record<string, string> = {
    COMPLETED: 'green',
    FAILED: 'red',
    IN_PROGRESS: 'blue',
    PENDING_APPROVAL: 'orange',
    PARTIAL_SUCCESS: 'gold',
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

function batchChildSummaryTitle(resourceName: string | undefined): string {
    return resourceName && resourceName.trim() !== '' ? resourceName : '—';
}

export default function VMBatchDetailPage() {
    const { t } = useTranslation(['vm', 'common']);
    const params = useParams<{ id: string }>();
    const router = useRouter();
    const batchId = params.id;
    const { messageApi, messageContextHolder } = useMessage();

    const { data: batchStatus, isLoading, refetch } = useApiGet<BatchJobStatus>(
        ['vm-batch-detail', batchId],
        () => api.GET('/vms/batch/{batch_id}', { params: { path: { batch_id: batchId } } })
    );

    const retryMutation = useApiMutation(
        () => api.POST('/vms/batch/{batch_id}/retry', { params: { path: { batch_id: batchId } } }),
        {
            onSuccess: () => {
                void messageApi.success(t('batch.retry_submitted'));
                void refetch();
            },
        }
    );

    const cancelMutation = useApiMutation(
        () => api.POST('/vms/batch/{batch_id}/cancel', { params: { path: { batch_id: batchId } } }),
        {
            onSuccess: () => {
                void messageApi.success(t('batch.cancel_submitted'));
                void refetch();
            },
        }
    );

    const status = batchStatus?.status;
    const canRetry = status === 'FAILED' || status === 'PARTIAL_SUCCESS';
    const canCancel = status === 'PENDING_APPROVAL' || status === 'IN_PROGRESS';
    const summaryItems: DescriptionsProps['items'] = [
        {
            key: 'status',
            label: t('batch.status'),
            children: (
                <Tag color={STATUS_COLORS[status ?? ''] ?? 'default'}>
                    {status ? batchStatusLabel(status, t) : '—'}
                </Tag>
            ),
        },
        {
            key: 'operation',
            label: t('batch.operation'),
            children: batchStatus?.operation ? batchOperationLabel(batchStatus.operation, t) : '—',
        },
        {
            key: 'childCount',
            label: t('batch.child_count'),
            children: batchStatus?.child_count ?? 0,
        },
        {
            key: 'successCount',
            label: t('batch.success_count'),
            children: <Text type="success">{batchStatus?.success_count ?? 0}</Text>,
        },
        {
            key: 'failedCount',
            label: t('batch.failed_count'),
            children: <Text type="danger">{batchStatus?.failed_count ?? 0}</Text>,
        },
        {
            key: 'pendingCount',
            label: t('batch.pending_count'),
            children: batchStatus?.pending_count ?? 0,
        },
    ];

    const childColumns = [
        {
            title: t('batch.child.resource'),
            key: 'resource_summary',
            render: (_: unknown, record: NonNullable<BatchJobStatus['children']>[number]) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{batchChildSummaryTitle(record.resource_name)}</Text>
                    <Text copyable={{ text: record.ticket_id }} type="secondary" style={{ fontSize: 13 }}>
                        {t('batch.child.ticket')}: {formatRecordID(record.ticket_id)}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('batch.child.status'),
            dataIndex: 'status',
            key: 'status',
            width: 120,
            render: (s: string) => <Tag color={STATUS_COLORS[s] ?? 'default'}>{batchStatusLabel(s, t)}</Tag>,
        },
        { title: t('batch.child.attempt'), dataIndex: 'attempt_count', key: 'attempt_count', width: 90 },
        { title: t('batch.child.error'), dataIndex: 'last_error', key: 'last_error', ellipsis: true },
    ];

    return (
        <div data-testid="vm-batch-detail-page">
            {messageContextHolder}
            <PageHeader
                title={(
                    <Space size="small">
                        <ThunderboltOutlined style={{ color: '#fa8c16' }} />
                        <span>{t('batch.detail_title')}</span>
                    </Space>
                )}
                subtitle={t('batch.detail_subtitle')}
                actions={(
                    <Button
                        icon={<ArrowLeftOutlined />}
                        type="text"
                        onClick={() => router.push('/vms/batch')}
                    >
                        {t('batch.list_title')}
                    </Button>
                )}
            />

            <PageSurface loading={isLoading}>
                <div
                    role="status"
                    aria-live="polite"
                    data-testid="batch-status-live"
                    style={{ marginBottom: 12 }}
                >
                    <Text type="secondary">
                        {t('batch.live_status_summary', {
                            batch_id: batchId,
                            status: batchStatus?.status ? batchStatusLabel(batchStatus.status, t) : '—',
                            success_count: batchStatus?.success_count ?? 0,
                            failed_count: batchStatus?.failed_count ?? 0,
                            pending_count: batchStatus?.pending_count ?? 0,
                        })}
                    </Text>
                </div>
                <Descriptions bordered column={3} size="small" items={summaryItems} />

                <Space>
                    <Button
                        icon={<RedoOutlined />}
                        data-testid="batch-retry-button"
                        disabled={!canRetry}
                        loading={retryMutation.isPending}
                        onClick={() => retryMutation.mutate(undefined)}
                    >
                        {t('batch.retry_failed')}
                    </Button>
                    <Button
                        danger
                        icon={<StopOutlined />}
                        data-testid="batch-cancel-button"
                        disabled={!canCancel}
                        loading={cancelMutation.isPending}
                        onClick={() => cancelMutation.mutate(undefined)}
                    >
                        {t('batch.cancel_pending')}
                    </Button>
                </Space>
            </PageSurface>

            <PageSurface
                title={t('batch.child.title')}
                flush={true}
            >
                <Table
                    rowKey="ticket_id"
                    loading={isLoading}
                    dataSource={batchStatus?.children ?? []}
                    columns={childColumns}
                    pagination={false}
                    size="small"
                />
            </PageSurface>
        </div>
    );
}
