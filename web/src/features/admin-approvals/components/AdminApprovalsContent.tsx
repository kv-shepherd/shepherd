'use client';

import {
    Badge,
    Button,
    Card,
    Descriptions,
    Form,
    Input,
    Modal,
    Popconfirm,
    Select,
    Segmented,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    AuditOutlined,
    CheckCircleOutlined,
    CloseCircleOutlined,
    DeleteOutlined,
    ExclamationCircleOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { useAdminApprovalsController } from '../hooks/useAdminApprovalsController';
import {
    getPriorityTier,
    OP_TYPE_CONFIG,
    STATUS_BADGES,
    STATUS_COLORS,
    STATUS_FILTER_OPTIONS,
    type ApprovalStatus,
    type ApprovalTicket,
    type Cluster,
} from '../types';

const { Title, Text } = Typography;

export function AdminApprovalsContent() {
    const { t } = useTranslation(['approval', 'common']);
    const approvals = useAdminApprovalsController({ t });

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
                const Icon = config.icon;
                return (
                    <Tag color={config.color} icon={<Icon />}>
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
                if (record.status !== 'PENDING') {
                    return <Text type="secondary">—</Text>;
                }
                return (
                    <Space>
                        <Button
                            type="primary"
                            size="small"
                            icon={<CheckCircleOutlined />}
                            data-testid={`approval-action-approve-${record.id}`}
                            onClick={() => approvals.openApproveModal(record)}
                        >
                            {t('common:button.approve')}
                        </Button>
                        <Button
                            danger
                            size="small"
                            icon={<CloseCircleOutlined />}
                            data-testid={`approval-action-reject-${record.id}`}
                            onClick={() => approvals.openRejectModal(record)}
                        >
                            {t('common:button.reject')}
                        </Button>
                        <Popconfirm
                            title={t('cancel_confirm')}
                            onConfirm={() => approvals.submitCancel(record.id)}
                            okText={t('common:button.confirm')}
                            cancelText={t('common:button.cancel')}
                        >
                            <Button
                                size="small"
                                icon={<ExclamationCircleOutlined />}
                                data-testid={`approval-action-cancel-${record.id}`}
                                loading={approvals.cancelPending}
                            >
                                {t('cancel')}
                            </Button>
                        </Popconfirm>
                    </Space>
                );
            },
        },
    ];

    return (
        <div data-testid="admin-approvals-page">
            {approvals.messageContextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('title')}</Title>
                    <Text type="secondary">{t('subtitle')}</Text>
                </div>
                <Space>
                    <Segmented
                        data-testid="approvals-status-filter"
                        value={approvals.statusFilter}
                        onChange={(value) => approvals.changeStatusFilter(value as 'ALL' | ApprovalStatus)}
                        options={STATUS_FILTER_OPTIONS.map((option) => ({
                            label: t(option.i18nKey),
                            value: option.key,
                        }))}
                    />
                    <Button icon={<ReloadOutlined />} data-testid="approvals-refresh-btn" onClick={() => approvals.refetch()}>
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
                    dataSource={approvals.data?.items ?? []}
                    rowKey="id"
                    loading={approvals.isLoading}
                    rowClassName={(record) => {
                        if (record.status !== 'PENDING') {
                            return '';
                        }
                        const tier = getPriorityTier(record.created_at);
                        if (tier === 'urgent') {
                            return 'approval-row-urgent';
                        }
                        if (tier === 'warning') {
                            return 'approval-row-warning';
                        }
                        return '';
                    }}
                    pagination={{
                        current: approvals.page,
                        pageSize: approvals.pageSize,
                        total: approvals.data?.pagination?.total ?? 0,
                        showTotal: (total) => t('common:table.total', { total }),
                        onChange: (page, pageSize) => {
                            approvals.setPage(page);
                            approvals.setPageSize(pageSize);
                        },
                    }}
                    size="middle"
                />
            </Card>

            <Modal
                title={approvals.approveModal?.operation_type === 'DELETE'
                    ? t('approve_modal.delete_title')
                    : t('approve_modal.title')}
                open={Boolean(approvals.approveModal)}
                onOk={() => {
                    void approvals.submitApprove();
                }}
                onCancel={approvals.closeApproveModal}
                confirmLoading={approvals.approvePending}
                forceRender
                data-testid="approve-modal"
            >
                <Form form={approvals.approveForm} layout="vertical" name="approve-form">
                    {approvals.approveModal?.operation_type !== 'DELETE' && (
                        <>
                            <Form.Item
                                name="selected_cluster_id"
                                label={t('approve_modal.cluster')}
                                extra={t('approve_modal.cluster_hint')}
                            >
                                <Select
                                    placeholder={t('approve_modal.cluster')}
                                    options={approvals.clustersData?.items
                                        ?.filter((cluster: Cluster) => cluster.status === 'HEALTHY' && cluster.enabled !== false)
                                        .map((cluster: Cluster) => ({
                                            label: (
                                                <Space>
                                                    <Text strong>{cluster.display_name || cluster.name}</Text>
                                                    {cluster.kubevirt_version && <Tag color="blue">KV {cluster.kubevirt_version}</Tag>}
                                                </Space>
                                            ),
                                            value: cluster.id,
                                        }))}
                                />
                            </Form.Item>
                            <Form.Item name="selected_storage_class" label={t('approve_modal.storage_class')}>
                                <Input placeholder="e.g. rook-ceph-block" />
                            </Form.Item>
                        </>
                    )}
                    {approvals.approveModal?.operation_type === 'DELETE' && (
                        <div style={{ marginBottom: 16 }}>
                            <Descriptions
                                bordered
                                size="small"
                                column={1}
                                style={{ marginBottom: 12 }}
                            >
                                <Descriptions.Item label={t('approve_modal.delete_target_vm')}>
                                    <Text strong style={{ color: '#cf1322' }}>
                                        {approvals.approveModal.target_vm_name || '—'}
                                    </Text>
                                </Descriptions.Item>
                                <Descriptions.Item label={t('requester')}>
                                    {approvals.approveModal.requester}
                                </Descriptions.Item>
                                {approvals.approveModal.reason && (
                                    <Descriptions.Item label={t('reason')}>
                                        {approvals.approveModal.reason}
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

            <Modal
                title={t('reject_modal.title')}
                open={Boolean(approvals.rejectModal)}
                onOk={() => {
                    void approvals.submitReject();
                }}
                onCancel={approvals.closeRejectModal}
                confirmLoading={approvals.rejectPending}
                forceRender
                data-testid="reject-modal"
            >
                <Form form={approvals.rejectForm} layout="vertical" name="reject-form">
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
