'use client';

/**
 * Approvals workbench — admin approval management.
 *
 * OpenAPI: GET /approvals, POST /approvals/{ticket_id}/approve|reject
 * ADR-0017: Admin selects target cluster (ClusterID) on approval
 * Stage 5.B: Cluster capability matching, overcommit validation
 * Stage 5.D: DELETE tickets show target VM info
 */
import { useState } from 'react';
import {
    Table,
    Button,
    Space,
    Typography,
    Tag,
    Modal,
    Form,
    Input,
    Select,
    message,
    Card,
    Badge,
    Segmented,
    Descriptions,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    ReloadOutlined,
    CheckCircleOutlined,
    CloseCircleOutlined,
    AuditOutlined,
    DeleteOutlined,
    PlusCircleOutlined,
    ExclamationCircleOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type ApprovalTicket = components['schemas']['ApprovalTicket'];
type ApprovalTicketList = components['schemas']['ApprovalTicketList'];
type ApprovalDecisionRequest = components['schemas']['ApprovalDecisionRequest'];
type RejectDecisionRequest = components['schemas']['RejectDecisionRequest'];
type ClusterList = components['schemas']['ClusterList'];
type Cluster = components['schemas']['Cluster'];
type ApprovalStatus = NonNullable<ApprovalTicket['status']>;

const STATUS_COLORS: Record<string, string> = {
    PENDING: 'gold',
    APPROVED: 'green',
    REJECTED: 'red',
    CANCELLED: 'default',
    EXECUTING: 'blue',
    SUCCESS: 'green',
    FAILED: 'red',
};

const STATUS_BADGES: Record<string, 'processing' | 'success' | 'error' | 'default'> = {
    PENDING: 'processing',
    APPROVED: 'success',
    REJECTED: 'error',
    CANCELLED: 'default',
    EXECUTING: 'processing',
    SUCCESS: 'success',
    FAILED: 'error',
};

const OP_TYPE_CONFIG: Record<string, { color: string; icon: React.ReactNode }> = {
    CREATE: { color: 'blue', icon: <PlusCircleOutlined /> },
    DELETE: { color: 'red', icon: <DeleteOutlined /> },
};

/** ADR-0015 §11: Visual priority tier based on days pending. */
const getPriorityTier = (createdAt?: string): 'urgent' | 'warning' | 'normal' => {
    if (!createdAt) return 'normal';
    const days = dayjs().diff(dayjs(createdAt), 'day');
    if (days > 7) return 'urgent';   // 🔴 Red
    if (days > 3) return 'warning';  // 🟡 Yellow
    return 'normal';
};

export default function ApprovalsPage() {
    const { t } = useTranslation(['approval', 'common']);
    const [messageApi, contextHolder] = message.useMessage();
    const [statusFilter, setStatusFilter] = useState<'ALL' | ApprovalStatus>('PENDING');
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [approveModal, setApproveModal] = useState<ApprovalTicket | null>(null);
    const [rejectModal, setRejectModal] = useState<ApprovalTicket | null>(null);
    const [approveForm] = Form.useForm<ApprovalDecisionRequest>();
    const [rejectForm] = Form.useForm<RejectDecisionRequest>();

    // Fetch approvals
    const { data, isLoading, refetch } = useApiGet<ApprovalTicketList>(
        ['approvals', statusFilter, page, pageSize],
        () => api.GET('/approvals', {
            params: {
                query: statusFilter === 'ALL'
                    ? { page, per_page: pageSize }
                    : { status: statusFilter, page, per_page: pageSize },
            },
        })
    );

    // Fetch clusters for approval modal — only needed for CREATE tickets (ADR-0017).
    const isCreateTicket = approveModal?.operation_type !== 'DELETE';
    const { data: clustersData } = useApiGet<ClusterList>(
        ['admin-clusters', 'approval-select'],
        () => api.GET('/admin/clusters'),
        { enabled: !!approveModal && isCreateTicket }
    );

    // Approve mutation
    const approveMutation = useApiMutation<
        { ticketId: string; body: ApprovalDecisionRequest },
        unknown
    >(
        ({ ticketId, body }) => api.POST('/approvals/{ticket_id}/approve', {
            params: { path: { ticket_id: ticketId } },
            body,
        }),
        {
            invalidateKeys: [['approvals'], ['vms']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setApproveModal(null);
                approveForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    // Reject mutation
    const rejectMutation = useApiMutation<
        { ticketId: string; body: RejectDecisionRequest },
        unknown
    >(
        ({ ticketId, body }) => api.POST('/approvals/{ticket_id}/reject', {
            params: { path: { ticket_id: ticketId } },
            body,
        }),
        {
            invalidateKeys: [['approvals']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setRejectModal(null);
                rejectForm.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
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
                    <Text copyable style={{ fontSize: 12 }}>{id.slice(0, 8)}</Text>
                </Space>
            ),
        },
        {
            title: t('operation_type'),
            dataIndex: 'operation_type',
            key: 'operation_type',
            width: 110,
            render: (opType: ApprovalTicket['operation_type']) => {
                const config = OP_TYPE_CONFIG[opType ?? 'CREATE'] ?? OP_TYPE_CONFIG.CREATE;
                return (
                    <Tag color={config.color} icon={config.icon}>
                        {t(`op_type.${opType ?? 'CREATE'}`)}
                    </Tag>
                );
            },
        },
        {
            title: t('target_vm'),
            key: 'target_vm',
            width: 160,
            render: (_, record) => {
                if (record.operation_type === 'DELETE' && record.target_vm_name) {
                    return (
                        <Space>
                            <DeleteOutlined style={{ color: '#cf1322' }} />
                            <Text strong style={{ color: '#cf1322' }}>
                                {record.target_vm_name}
                            </Text>
                        </Space>
                    );
                }
                return <Text type="secondary">—</Text>;
            },
        },
        {
            title: t('common:table.status'),
            dataIndex: 'status',
            key: 'status',
            width: 120,
            render: (status: ApprovalTicket['status']) => (
                <Badge
                    status={STATUS_BADGES[status] ?? 'default'}
                    text={<Tag color={STATUS_COLORS[status]}>{t(`status.${status}`)}</Tag>}
                />
            ),
        },
        {
            title: t('requester'),
            dataIndex: 'requester',
            key: 'requester',
            width: 140,
        },
        {
            title: t('reason'),
            dataIndex: 'reason',
            key: 'reason',
            ellipsis: true,
            render: (reason: string) => <Text type="secondary">{reason || '—'}</Text>,
        },
        {
            title: t('approver'),
            dataIndex: 'approver',
            key: 'approver',
            width: 140,
            render: (approver: string) => <Text type="secondary">{approver || '—'}</Text>,
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => (
                <Text type="secondary">{date ? dayjs(date).format('YYYY-MM-DD HH:mm') : '—'}</Text>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 160,
            render: (_, record) => {
                if (record.status !== 'PENDING') return <Text type="secondary">—</Text>;
                return (
                    <Space>
                        <Button
                            type="primary"
                            size="small"
                            icon={<CheckCircleOutlined />}
                            onClick={() => setApproveModal(record)}
                        >
                            {t('common:button.approve')}
                        </Button>
                        <Button
                            danger
                            size="small"
                            icon={<CloseCircleOutlined />}
                            onClick={() => setRejectModal(record)}
                        >
                            {t('common:button.reject')}
                        </Button>
                    </Space>
                );
            },
        },
    ];

    return (
        <div>
            {contextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('title')}</Title>
                    <Text type="secondary">{t('subtitle')}</Text>
                </div>
                <Space>
                    <Segmented
                        value={statusFilter}
                        onChange={(val) => { setStatusFilter(val as 'ALL' | ApprovalStatus); setPage(1); }}
                        options={[
                            { label: t('filter.pending'), value: 'PENDING' },
                            { label: t('filter.executing'), value: 'EXECUTING' },
                            { label: t('filter.success'), value: 'SUCCESS' },
                            { label: t('filter.failed'), value: 'FAILED' },
                            { label: t('filter.approved'), value: 'APPROVED' },
                            { label: t('filter.rejected'), value: 'REJECTED' },
                            { label: t('filter.cancelled'), value: 'CANCELLED' },
                            { label: t('filter.all'), value: 'ALL' },
                        ]}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                {/* ADR-0015 §11: Priority tier highlighting styles */}
                <style>{`
                    .approval-row-urgent td { background-color: rgba(255, 77, 79, 0.06) !important; }
                    .approval-row-warning td { background-color: rgba(250, 173, 20, 0.06) !important; }
                `}</style>
                <Table<ApprovalTicket>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    rowClassName={(record) => {
                        if (record.status !== 'PENDING') return '';
                        const tier = getPriorityTier(record.created_at);
                        if (tier === 'urgent') return 'approval-row-urgent';
                        if (tier === 'warning') return 'approval-row-warning';
                        return '';
                    }}
                    pagination={{
                        current: page,
                        pageSize,
                        total: data?.pagination?.total ?? 0,
                        showTotal: (total) => t('common:table.total', { total }),
                        onChange: (p, ps) => { setPage(p); setPageSize(ps); },
                    }}
                    size="middle"
                />
            </Card>

            {/* Approve Modal — conditionally shows cluster fields for CREATE, simplified for DELETE */}
            <Modal
                title={approveModal?.operation_type === 'DELETE'
                    ? t('approve_modal.delete_title')
                    : t('approve_modal.title')}
                open={!!approveModal}
                onOk={() => {
                    approveForm.validateFields().then((values) => {
                        if (approveModal) {
                            approveMutation.mutate({ ticketId: approveModal.id, body: values });
                        }
                    });
                }}
                onCancel={() => { setApproveModal(null); approveForm.resetFields(); }}
                confirmLoading={approveMutation.isPending}
                forceRender
            >
                <Form form={approveForm} layout="vertical" name="approve-form">
                    {/* Show cluster selection only for CREATE tickets (ADR-0017) */}
                    {approveModal?.operation_type !== 'DELETE' && (
                        <>
                            <Form.Item
                                name="selected_cluster_id"
                                label={t('approve_modal.cluster')}
                                extra={t('approve_modal.cluster_hint')}
                            >
                                <Select
                                    placeholder={t('approve_modal.cluster')}
                                    options={clustersData?.items
                                        ?.filter((c: Cluster) => c.status === 'HEALTHY' && c.enabled !== false)
                                        .map((c: Cluster) => ({
                                            label: (
                                                <Space>
                                                    <Text strong>{c.display_name || c.name}</Text>
                                                    {c.kubevirt_version && <Tag color="blue">KV {c.kubevirt_version}</Tag>}
                                                </Space>
                                            ),
                                            value: c.id,
                                        }))}
                                />
                            </Form.Item>
                            <Form.Item name="selected_storage_class" label={t('approve_modal.storage_class')}>
                                <Input placeholder="e.g. rook-ceph-block" />
                            </Form.Item>
                        </>
                    )}
                    {/* DELETE ticket — show target VM info (Stage 5.D) */}
                    {approveModal?.operation_type === 'DELETE' && (
                        <div style={{ marginBottom: 16 }}>
                            <Descriptions
                                bordered
                                size="small"
                                column={1}
                                style={{ marginBottom: 12 }}
                            >
                                <Descriptions.Item label={t('approve_modal.delete_target_vm')}>
                                    <Text strong style={{ color: '#cf1322' }}>
                                        {approveModal.target_vm_name || '—'}
                                    </Text>
                                </Descriptions.Item>
                                <Descriptions.Item label={t('requester')}>
                                    {approveModal.requester}
                                </Descriptions.Item>
                                {approveModal.reason && (
                                    <Descriptions.Item label={t('reason')}>
                                        {approveModal.reason}
                                    </Descriptions.Item>
                                )}
                            </Descriptions>
                            <div style={{
                                padding: '12px 16px',
                                background: '#fff2e8',
                                border: '1px solid #ffbb96',
                                borderRadius: 8,
                                display: 'flex',
                                alignItems: 'flex-start',
                                gap: 8,
                            }}>
                                <ExclamationCircleOutlined style={{ color: '#d4380d', marginTop: 2 }} />
                                <Text type="warning">{t('approve_modal.delete_warning')}</Text>
                            </div>
                        </div>
                    )}
                    <Form.Item name="comment" label={t('approve_modal.comment')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                </Form>
            </Modal>

            {/* Reject Modal */}
            <Modal
                title={t('reject_modal.title')}
                open={!!rejectModal}
                onOk={() => {
                    rejectForm.validateFields().then((values) => {
                        if (rejectModal) {
                            rejectMutation.mutate({ ticketId: rejectModal.id, body: values });
                        }
                    });
                }}
                onCancel={() => { setRejectModal(null); rejectForm.resetFields(); }}
                confirmLoading={rejectMutation.isPending}
                forceRender
            >
                <Form form={rejectForm} layout="vertical" name="reject-form">
                    <Form.Item
                        name="reason"
                        label={t('reject_modal.reason')}
                        rules={[{ required: true, message: 'Rejection reason is required' }]}
                    >
                        <Input.TextArea
                            rows={4}
                            placeholder={t('reject_modal.reason_placeholder')}
                        />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
