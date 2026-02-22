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
import { Button, Card, Descriptions, Divider, Space, Table, Tag, Typography } from 'antd';
import { ArrowLeftOutlined, RedoOutlined, StopOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { useParams, useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { useApiGet } from '@/lib/api/useApiGet';
import { useApiMutation } from '@/lib/api/useApiMutation';
import { api } from '@/lib/api/client';
import { useMessage } from '@/lib/hooks/useMessage';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type BatchJobStatus = components['schemas']['VMBatchStatusResponse'];

const STATUS_COLORS: Record<string, string> = {
    COMPLETED: 'green',
    FAILED: 'red',
    IN_PROGRESS: 'blue',
    PENDING_APPROVAL: 'orange',
    PARTIAL_SUCCESS: 'gold',
    CANCELLED: 'default',
};

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
                void messageApi.success(t('batch.retry_submitted', { defaultValue: 'Retry submitted.' }));
                void refetch();
            },
        }
    );

    const cancelMutation = useApiMutation(
        () => api.POST('/vms/batch/{batch_id}/cancel', { params: { path: { batch_id: batchId } } }),
        {
            onSuccess: () => {
                void messageApi.success(t('batch.cancel_submitted', { defaultValue: 'Batch cancelled.' }));
                void refetch();
            },
        }
    );

    const status = batchStatus?.status;
    const canRetry = status === 'FAILED' || status === 'PARTIAL_SUCCESS';
    const canCancel = status === 'PENDING_APPROVAL' || status === 'IN_PROGRESS';

    const childColumns = [
        { title: t('batch.child.ticket'), dataIndex: 'ticket_id', key: 'ticket_id', width: 150 },
        { title: t('batch.child.resource'), dataIndex: 'resource_name', key: 'resource_name' },
        {
            title: t('batch.child.status'),
            dataIndex: 'status',
            key: 'status',
            width: 120,
            render: (s: string) => <Tag color={STATUS_COLORS[s] ?? 'default'}>{s}</Tag>,
        },
        { title: t('batch.child.attempt'), dataIndex: 'attempt_count', key: 'attempt_count', width: 90 },
        { title: t('batch.child.error'), dataIndex: 'last_error', key: 'last_error', ellipsis: true },
    ];

    return (
        <div data-testid="vm-batch-detail-page">
            {messageContextHolder}
            <div style={{ marginBottom: 24 }}>
                <Button
                    icon={<ArrowLeftOutlined />}
                    type="text"
                    onClick={() => router.push('/vms/batch')}
                >
                    {t('batch.list_title', { defaultValue: 'Batch Operations' })}
                </Button>
                <Title level={4} style={{ margin: '8px 0 0' }}>
                    <ThunderboltOutlined style={{ marginRight: 8, color: '#fa8c16' }} />
                    {t('batch.detail_title', { defaultValue: 'Batch Operation Detail' })}
                </Title>
            </div>

            <Card style={{ borderRadius: 12, marginBottom: 16 }} loading={isLoading}>
                <div
                    role="status"
                    aria-live="polite"
                    data-testid="batch-status-live"
                    style={{ marginBottom: 12 }}
                >
                    <Text type="secondary">
                        {t('batch.live_status_summary', {
                            batch_id: batchId,
                            status: batchStatus?.status ?? '—',
                            success_count: batchStatus?.success_count ?? 0,
                            failed_count: batchStatus?.failed_count ?? 0,
                            pending_count: batchStatus?.pending_count ?? 0,
                        })}
                    </Text>
                </div>
                <Descriptions bordered column={3} size="small">
                    <Descriptions.Item label={t('batch.status')}>
                        <Tag color={STATUS_COLORS[status ?? ''] ?? 'default'}>{status ?? '—'}</Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label={t('batch.operation')}>
                        {batchStatus?.operation ?? '—'}
                    </Descriptions.Item>
                    <Descriptions.Item label={t('batch.child_count')}>
                        {batchStatus?.child_count ?? 0}
                    </Descriptions.Item>
                    <Descriptions.Item label={t('batch.success_count')}>
                        <Text type="success">{batchStatus?.success_count ?? 0}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label={t('batch.failed_count')}>
                        <Text type="danger">{batchStatus?.failed_count ?? 0}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label={t('batch.pending_count')}>
                        {batchStatus?.pending_count ?? 0}
                    </Descriptions.Item>
                </Descriptions>

                <Divider />

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
            </Card>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table
                    rowKey="ticket_id"
                    loading={isLoading}
                    dataSource={batchStatus?.children ?? []}
                    columns={childColumns}
                    pagination={false}
                    size="small"
                />
            </Card>
        </div>
    );
}
