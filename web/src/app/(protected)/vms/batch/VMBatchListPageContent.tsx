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
import { Button, Space, Table, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    EyeOutlined,
    RedoOutlined,
    ReloadOutlined,
    StopOutlined,
    ThunderboltOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import type { TFunction } from 'i18next';
import { useRouter } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { useApiGet } from '@/lib/api/useApiGet';
import { useApiMutation } from '@/lib/api/useApiMutation';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import { useMessage } from '@/lib/hooks/useMessage';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';

const { Text } = Typography;

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

    const { data, isLoading, refetch } = useApiGet<BatchJobList>(
        ['vm-batch-list'],
        () => api.GET('/vms/batch', {}) as Promise<{ data?: BatchJobList; error?: unknown; response?: Response }>
    );

    const retryMutation = useApiMutation<unknown, string>(
        (batchId) => api.POST('/vms/batch/{batch_id}/retry', { params: { path: { batch_id: batchId } } }),
        {
            onSuccess: () => {
                void messageApi.success(t('batch.retry_submitted'));
                void refetch();
            },
            onError: (err) => {
                void messageApi.error(translateApiError(t, err));
            },
        }
    );

    const cancelMutation = useApiMutation<unknown, string>(
        (batchId) => api.POST('/vms/batch/{batch_id}/cancel', { params: { path: { batch_id: batchId } } }),
        {
            onSuccess: () => {
                void messageApi.success(t('batch.cancel_submitted'));
                void refetch();
            },
            onError: (err) => {
                void messageApi.error(translateApiError(t, err));
            },
        }
    );

    const canRetry = (status: string) => status === 'FAILED' || status === 'PARTIAL_SUCCESS';
    const canCancel = (status: string) =>
        status === 'PENDING_APPROVAL' || status === 'IN_PROGRESS';

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
                    <Text type="secondary" style={{ fontSize: 12 }}>
                        {t('batch.success_count')}: {record.success_count} · {t('batch.failed_count')}: {record.failed_count} · {t('batch.pending_count')}: {record.pending_count}
                    </Text>
                    <Text copyable={{ text: record.id }} type="secondary" style={{ fontSize: 12 }}>
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
                        aria-label={t('batch.retry_failed')}
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
                        aria-label={t('batch.cancel_pending')}
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
            <PageHeader
                title={(
                    <Space size="small">
                        <ThunderboltOutlined style={{ color: '#fa8c16' }} />
                        <span>{t('batch.list_title')}</span>
                    </Space>
                )}
                subtitle={t('batch.list_subtitle')}
                actions={(
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                )}
            />

            <PageSurface flush={true}>
                <Table<BatchJobSummary>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={false}
                    size="middle"
                />
            </PageSurface>
        </div>
    );
}
