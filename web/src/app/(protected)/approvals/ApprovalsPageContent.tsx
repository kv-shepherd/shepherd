'use client';

/**
 * /approvals — User's own approval request list.
 * master-flow.md §7: User Self-Service — My Approvals.
 *
 * API contracts:
 *   GET  /approvals?status={filter}                 → ApprovalTicketList
 *   POST /approvals/{ticket_id}/cancel              → 204 (cancel own request)
 *
 * E2E data-testid requirements:
 *   approvals-page
 *   approval-action-cancel-{id}
 *   approvals-status-filter
 */
import { Badge, Button, Card, Segmented, Space, Table, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    AuditOutlined,
    CloseCircleOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useApiGet } from '@/lib/api/useApiGet';
import { useApiMutation } from '@/lib/api/useApiMutation';
import { api } from '@/lib/api/client';
import { useMessage } from '@/lib/hooks/useMessage';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';

const { Title, Text } = Typography;

type ApprovalStatus = 'PENDING' | 'APPROVED' | 'REJECTED' | 'CANCELLED';

interface ApprovalTicket {
    id: string;
    operation_type?: string;
    status: ApprovalStatus;
    requester: string;
    reason?: string;
    approver?: string;
    created_at: string;
    updated_at?: string;
}

interface ApprovalTicketList {
    items: ApprovalTicket[];
    pagination?: { total: number; page: number; per_page: number };
}

const STATUS_COLORS: Record<ApprovalStatus, string> = {
    PENDING: 'orange',
    APPROVED: 'green',
    REJECTED: 'red',
    CANCELLED: 'default',
};

export default function ApprovalsPage() {
    const { t } = useTranslation(['approval', 'common']);
    const [statusFilter, setStatusFilter] = useState<'ALL' | ApprovalStatus>('ALL');
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const { messageApi, messageContextHolder } = useMessage();

    const { data, isLoading, refetch } = useApiGet<ApprovalTicketList>(
        ['my-approvals', statusFilter, page, pageSize],
        () =>
            // Spec: GET /approvals returns ApprovalTicketList; cast items to our local type
            api.GET('/approvals', {
                params: {
                    query: {
                        page,
                        per_page: pageSize,
                        ...(statusFilter !== 'ALL' ? { status: statusFilter as never } : {}),
                    },
                },
            }) as Promise<{ data?: ApprovalTicketList; error?: unknown; response?: Response }>
    );

    const cancelMutation = useApiMutation(
        (id: string) =>
            api.POST('/approvals/{ticket_id}/cancel', { params: { path: { ticket_id: id } } }),
        {
            onSuccess: () => {
                void messageApi.success(t('cancel_success', { defaultValue: 'Request cancelled.' }));
                void refetch();
            },
        }
    );

    const columns: ColumnsType<ApprovalTicket> = [
        {
            title: t('ticket_id'),
            dataIndex: 'id',
            key: 'id',
            width: 120,
            render: (id: string) => (
                <Space>
                    <AuditOutlined style={{ color: '#d4380d' }} />
                    <Text copyable style={{ fontSize: 12 }}>
                        {id.slice(0, 8)}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('operation_type'),
            dataIndex: 'operation_type',
            key: 'operation_type',
            width: 110,
            render: (opType: string) => <Tag color="purple">{opType ?? '—'}</Tag>,
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
            render: (reason: string) => <Text type="secondary">{reason || '—'}</Text>,
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => (
                <Text type="secondary">
                    <LocalDateTimeText value={date} />
                </Text>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 120,
            render: (_, record) => {
                if (record.status !== 'PENDING') {
                    return <Text type="secondary">—</Text>;
                }
                return (
                    <Button
                        size="small"
                        danger
                        icon={<CloseCircleOutlined />}
                        data-testid={`approval-action-cancel-${record.id}`}
                        loading={cancelMutation.isPending}
                        onClick={() => cancelMutation.mutate(record.id)}
                    >
                        {t('cancel')}
                    </Button>
                );
            },
        },
    ];

    return (
        <div data-testid="approvals-page">
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
                        {t('my_approvals_title', { defaultValue: 'My Approval Requests' })}
                    </Title>
                    <Text type="secondary">
                        {t('my_approvals_subtitle', { defaultValue: 'Track the status of your VM requests.' })}
                    </Text>
                </div>
                <Space>
                    <Segmented
                        data-testid="approvals-status-filter"
                        value={statusFilter}
                        onChange={(v) => {
                            setStatusFilter(v as 'ALL' | ApprovalStatus);
                            setPage(1);
                        }}
                        options={[
                            { label: t('filter_all', { defaultValue: 'All' }), value: 'ALL' },
                            { label: t('status.PENDING'), value: 'PENDING' },
                            { label: t('status.APPROVED'), value: 'APPROVED' },
                            { label: t('status.REJECTED'), value: 'REJECTED' },
                        ]}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<ApprovalTicket>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={{
                        current: page,
                        pageSize,
                        total: data?.pagination?.total ?? 0,
                        showTotal: (total) => t('common:table.total', { total }),
                        onChange: (p, ps) => {
                            setPage(p);
                            setPageSize(ps);
                        },
                    }}
                    size="middle"
                />
            </Card>
        </div>
    );
}
