'use client';

/**
 * /vms/batch — Batch VM Operation Status list page.
 * master-flow.md §5.4: Batch Power Operations.
 *
 * API contracts:
 *   GET /vms/batch                  → BatchJobList
 *
 * E2E data-testid requirements:
 *   vm-batch-page
 *   batch-action-detail-{id}
 */
import { Button, Card, Space, Table, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    EyeOutlined,
    RedoOutlined,
    ReloadOutlined,
    StopOutlined,
    ThunderboltOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { useApiGet } from '@/lib/api/useApiGet';
import { useApiMutation } from '@/lib/api/useApiMutation';
import { api } from '@/lib/api/client';
import { useMessage } from '@/lib/hooks/useMessage';

const { Title, Text } = Typography;

interface BatchJobSummary {
    id: string;
    operation: string;
    status: string;
    child_count: number;
    success_count: number;
    failed_count: number;
    pending_count: number;
    created_at: string;
}

interface BatchJobList {
    items: BatchJobSummary[];
    pagination?: { total: number; page: number; per_page: number };
}

const STATUS_COLORS: Record<string, string> = {
    COMPLETED: 'green',
    FAILED: 'red',
    IN_PROGRESS: 'blue',
    PENDING_APPROVAL: 'orange',
    PARTIAL_SUCCESS: 'warning',
    CANCELLED: 'default',
};

export default function VMBatchListPage() {
    const { t } = useTranslation(['vm', 'common']);
    const router = useRouter();
    const { messageApi, messageContextHolder } = useMessage();

    const { data, isLoading, refetch } = useApiGet<BatchJobList>(
        ['vm-batch-list'],
        () => api.GET('/vms/batch', {}) as Promise<{ data?: BatchJobList; error?: unknown; response?: Response }>
    );

    const retryMutation = useApiMutation<unknown, string>(
        (batchId) => api.POST('/vms/batch/{batch_id}/retry', { params: { path: { batch_id: batchId } } }),
        {
            onSuccess: () => {
                void messageApi.success(t('batch.retry_submitted', { defaultValue: 'Retry submitted.' }));
                void refetch();
            },
            onError: (err) => {
                void messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const cancelMutation = useApiMutation<unknown, string>(
        (batchId) => api.POST('/vms/batch/{batch_id}/cancel', { params: { path: { batch_id: batchId } } }),
        {
            onSuccess: () => {
                void messageApi.success(t('batch.cancel_submitted', { defaultValue: 'Batch cancelled.' }));
                void refetch();
            },
            onError: (err) => {
                void messageApi.error(err.message || t('common:message.error'));
            },
        }
    );

    const canRetry = (status: string) => status === 'FAILED' || status === 'PARTIAL_SUCCESS';
    const canCancel = (status: string) =>
        status === 'PENDING_APPROVAL' || status === 'PENDING' || status === 'IN_PROGRESS';

    const columns: ColumnsType<BatchJobSummary> = [
        {
            title: t('batch.id', { defaultValue: 'Batch ID' }),
            dataIndex: 'id',
            key: 'id',
            width: 150,
            render: (id: string) => <Text code style={{ fontSize: 12 }}>{id.slice(0, 12)}</Text>,
        },
        {
            title: t('batch.operation'),
            dataIndex: 'operation',
            key: 'operation',
            width: 120,
            render: (op: string) => <Tag color="purple">{op}</Tag>,
        },
        {
            title: t('batch.status'),
            dataIndex: 'status',
            key: 'status',
            width: 140,
            render: (status: string) => (
                <Tag color={STATUS_COLORS[status] ?? 'default'}>{status}</Tag>
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
            render: (date: string) => (
                <Text type="secondary">{dayjs(date).format('YYYY-MM-DD HH:mm')}</Text>
            ),
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
                        aria-label={t('batch.retry_failed', { defaultValue: 'Retry failed children' })}
                        data-testid={`batch-action-retry-${record.id}`}
                        disabled={!canRetry(record.status)}
                        loading={retryMutation.isPending}
                        onClick={() => retryMutation.mutate(record.id)}
                    />
                    <Button
                        type="text"
                        size="small"
                        danger
                        icon={<StopOutlined />}
                        aria-label={t('batch.cancel_pending', { defaultValue: 'Cancel pending children' })}
                        data-testid={`batch-action-cancel-${record.id}`}
                        disabled={!canCancel(record.status)}
                        loading={cancelMutation.isPending}
                        onClick={() => cancelMutation.mutate(record.id)}
                    />
                    <Button
                        type="text"
                        size="small"
                        icon={<EyeOutlined />}
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
            <div
                style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    marginBottom: 24,
                }}
            >
                <div>
                    <Title level={4} style={{ margin: 0 }}>
                        <ThunderboltOutlined style={{ marginRight: 8, color: '#fa8c16' }} />
                        {t('batch.list_title', { defaultValue: 'Batch Operations' })}
                    </Title>
                    <Text type="secondary">
                        {t('batch.list_subtitle', { defaultValue: 'History of batch VM power operations.' })}
                    </Text>
                </div>
                <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<BatchJobSummary>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={false}
                    size="middle"
                />
            </Card>
        </div>
    );
}
