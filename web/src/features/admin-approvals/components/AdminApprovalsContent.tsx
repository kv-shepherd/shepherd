'use client';

import {
    Alert,
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
    Switch,
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
import { UnitInputNumber } from '@/components/form/UnitInputNumber';
import {
    getPriorityTier,
    OPERATION_FILTER_OPTIONS,
    OP_TYPE_CONFIG,
    STATUS_BADGES,
    STATUS_COLORS,
    STATUS_FILTER_OPTIONS,
    type ApprovalStatus,
    type ApprovalTicket,
    type Cluster,
} from '../types';

const { Title, Text } = Typography;

/** Safely convert an unknown ticket_payload field to a displayable string. */
function toStr(value: unknown): string {
    if (value === null || value === undefined) return '—';
    if (typeof value === 'string') return value || '—';
    return String(value);
}

function getProvisioningPhaseTagColor(phase?: string): string {
    if (!phase) return 'default';
    if (phase === 'Succeeded' || phase === 'Ready') return 'green';
    if (phase === 'Failed') return 'red';
    return 'blue';
}

function getCloneTypeTagColor(cloneType?: string): string {
    if (!cloneType) return 'default';
    if (cloneType === 'copy') return 'orange';
    return 'geekblue';
}

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
            title: t('selected_cluster', 'Selected Cluster'),
            key: 'selected_cluster',
            width: 180,
            render: (_, record) => {
                const placement = record.placement_evaluation;
                if (!placement?.selected_cluster_id) {
                    return <Text type="secondary">—</Text>;
                }
                const displayName = placement.selected_cluster_name || placement.selected_cluster_id;
                return (
                    <Space direction="vertical" size={0}>
                        <Text strong>{displayName}</Text>
                        {placement.advisory_code && (
                            <Text type="warning" style={{ fontSize: 12 }}>
                                {placement.advisory_code}
                            </Text>
                        )}
                        {placement.selected_cluster_name && placement.selected_cluster_name !== placement.selected_cluster_id && (
                            <Text type="secondary" style={{ fontSize: 12 }}>
                                {placement.selected_cluster_id}
                            </Text>
                        )}
                    </Space>
                );
            },
        },
        {
            title: t('approve_modal.provisioning.title', 'Provisioning Status'),
            key: 'provisioning',
            width: 190,
            render: (_, record) => {
                if (record.operation_type !== 'CREATE' || !record.provisioning) {
                    return <Text type="secondary">—</Text>;
                }
                const provisioning = record.provisioning;
                return (
                    <Space direction="vertical" size={2} data-testid={`approval-provisioning-summary-${record.id}`}>
                        <Tag color={getProvisioningPhaseTagColor(provisioning.phase)}>
                            {provisioning.phase || '—'}
                        </Tag>
                        {provisioning.clone_type === 'copy' && (
                            <Tag color={getCloneTypeTagColor(provisioning.clone_type)}>
                                {t('approve_modal.provisioning.clone_type_copy', 'Host-assisted copy')}
                            </Tag>
                        )}
                        {provisioning.failure_message ? (
                            <Text type="danger" style={{ fontSize: 12 }}>
                                {provisioning.failure_message}
                            </Text>
                        ) : provisioning.progress ? (
                            <Text type="secondary" style={{ fontSize: 12 }}>
                                {provisioning.progress}
                            </Text>
                        ) : null}
                    </Space>
                );
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
            <Card style={{ borderRadius: 12, marginBottom: 16 }}>
                <Space wrap size={12}>
                    <Select
                        value={approvals.operationFilter}
                        onChange={(value) => approvals.changeOperationFilter(value as 'ALL' | ApprovalTicket['operation_type'])}
                        options={OPERATION_FILTER_OPTIONS.map((option) => ({
                            label: t(option.i18nKey),
                            value: option.key,
                        }))}
                        style={{ minWidth: 180 }}
                        placeholder={t('filter.operation_label', 'Operation')}
                    />
                    <Select
                        value={approvals.placementSnapshotFilter}
                        onChange={(value) => approvals.changePlacementSnapshotFilter(value as 'ALL' | 'present' | 'missing')}
                        options={[
                            { label: t('filter.placement_all', 'All placement states'), value: 'ALL' },
                            { label: t('filter.placement_present', 'Placement captured'), value: 'present' },
                            { label: t('filter.placement_missing', 'Placement missing'), value: 'missing' },
                        ]}
                        style={{ minWidth: 200 }}
                        placeholder={t('filter.placement_label', 'Placement snapshot')}
                    />
                    <Input
                        allowClear
                        value={approvals.selectedClusterFilter}
                        onChange={(event) => approvals.changeSelectedClusterFilter(event.target.value)}
                        placeholder={t('filter.selected_cluster', 'Filter by cluster ID')}
                        style={{ width: 240 }}
                    />
                    <Input
                        allowClear
                        value={approvals.placementAdvisoryFilter}
                        onChange={(event) => approvals.changePlacementAdvisoryFilter(event.target.value)}
                        placeholder={t('filter.placement_advisory', 'Filter by placement advisory')}
                        style={{ width: 260 }}
                    />
                </Space>
            </Card>

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
                    {approvals.approveModal?.operation_type === 'CREATE' ? (() => {
                        const payload = approvals.approveModal?.ticket_payload as Record<string, unknown> | undefined;
                        const provisioning = approvals.approveModal?.provisioning;
                        return (
                            <>
                                {payload && (
                                    <Descriptions
                                        bordered
                                        size="small"
                                        column={1}
                                        style={{ marginBottom: 16 }}
                                        title={t('approve_modal.request_details', 'Request Details')}
                                    >
                                        <Descriptions.Item label={t('approve_modal.namespace', 'Namespace')}>{toStr(payload.namespace)}</Descriptions.Item>
                                        <Descriptions.Item label={t('approve_modal.template', 'Template ID')}>{toStr(payload.template_id)}</Descriptions.Item>
                                        <Descriptions.Item label={t('approve_modal.instance_size', 'Instance Size ID')}>{toStr(payload.instance_size_id)}</Descriptions.Item>
                                        <Descriptions.Item label={t('approve_modal.dedicated_cpu', 'Dedicated CPU')}>
                                            {payload.dedicated_cpu ? <Tag color="blue">{t('common:yes', 'Yes')}</Tag> : <Text type="secondary">{t('common:no', 'No')}</Text>}
                                        </Descriptions.Item>
                                    </Descriptions>
                                )}
                                {provisioning && (
                                    <Card
                                        size="small"
                                        title={t('approve_modal.provisioning.title', 'Provisioning Status')}
                                        style={{ marginBottom: 16 }}
                                        data-testid="approval-provisioning-card"
                                    >
                                        <Descriptions bordered size="small" column={1}>
                                            <Descriptions.Item label={t('approve_modal.provisioning.phase', 'Phase')}>
                                                <Tag color={getProvisioningPhaseTagColor(provisioning.phase)} data-testid="approval-provisioning-phase">
                                                    {provisioning.phase || '—'}
                                                </Tag>
                                            </Descriptions.Item>
                                            <Descriptions.Item label={t('approve_modal.provisioning.progress', 'Progress')}>
                                                {provisioning.progress || '—'}
                                            </Descriptions.Item>
                                            <Descriptions.Item label={t('approve_modal.provisioning.root_claim', 'Root Claim')}>
                                                {provisioning.claim_name || '—'}
                                            </Descriptions.Item>
                                            <Descriptions.Item label={t('approve_modal.provisioning.pvc_phase', 'PVC Phase')}>
                                                {provisioning.pvc_phase || '—'}
                                            </Descriptions.Item>
                                            <Descriptions.Item label={t('approve_modal.provisioning.clone_type', 'Clone Type')}>
                                                {provisioning.clone_type ? (
                                                    <Tag color={getCloneTypeTagColor(provisioning.clone_type)} data-testid="approval-provisioning-clone-type">
                                                        {provisioning.clone_type === 'copy'
                                                            ? t('approve_modal.provisioning.clone_type_copy', 'Host-assisted copy')
                                                            : provisioning.clone_type}
                                                    </Tag>
                                                ) : '—'}
                                            </Descriptions.Item>
                                            <Descriptions.Item label={t('approve_modal.provisioning.clone_phase', 'Clone Phase')}>
                                                {provisioning.clone_phase || '—'}
                                            </Descriptions.Item>
                                        </Descriptions>
                                        {provisioning.clone_fallback_reason && (
                                            <Alert
                                                type="warning"
                                                showIcon
                                                style={{ marginTop: 12 }}
                                                message={t('approve_modal.provisioning.clone_fallback_reason', 'Clone fallback reason')}
                                                description={provisioning.clone_fallback_reason}
                                            />
                                        )}
                                        {provisioning.failure_message && (
                                            <Alert
                                                type="error"
                                                showIcon
                                                style={{ marginTop: 12 }}
                                                message={t('approve_modal.provisioning.failure_message', 'Provisioning failure')}
                                                description={provisioning.failure_message}
                                            />
                                        )}
                                    </Card>
                                )}
                                <Form.Item
                                    name="selected_cluster_id"
                                    label={t('approve_modal.cluster')}
                                    extra={t('approve_modal.cluster_hint')}
                                >
                                    <Select
                                        placeholder={t('approve_modal.cluster')}
                                        options={approvals.clustersData?.items?.map((cluster: Cluster) => {
                                            const compatible = cluster.compatibility?.eligible !== false;
                                            const disabled = cluster.enabled === false || !compatible;
                                            return {
                                                label: (
                                                    <div>
                                                        <Space wrap>
                                                            <Text strong>{cluster.display_name || cluster.name}</Text>
                                                            {cluster.kubevirt_version && <Tag color="blue">KV {cluster.kubevirt_version}</Tag>}
                                                            {!compatible && (
                                                                <Tag color="red">
                                                                    {t('approve_modal.cluster_incompatible', 'Incompatible')}
                                                                </Tag>
                                                            )}
                                                            {compatible && cluster.compatibility?.advisory_code && (
                                                                <Tag color="orange">
                                                                    {t('approve_modal.cluster_advisory', 'Clone fallback likely')}
                                                                </Tag>
                                                            )}
                                                        </Space>
                                                        {compatible && cluster.compatibility?.advisory_message && (
                                                            <div style={{ marginTop: 4 }}>
                                                                <Text type="warning" style={{ fontSize: 12 }}>
                                                                    {cluster.compatibility.advisory_message}
                                                                </Text>
                                                            </div>
                                                        )}
                                                        {!compatible && cluster.compatibility?.reason_message && (
                                                            <div style={{ marginTop: 4 }}>
                                                                <Text type="secondary" style={{ fontSize: 12 }}>
                                                                    {cluster.compatibility.reason_message}
                                                                </Text>
                                                            </div>
                                                        )}
                                                    </div>
                                                ),
                                                value: cluster.id,
                                                disabled,
                                            };
                                        })}
                                    />
                                </Form.Item>
                                <Form.Item name="selected_storage_class" label={t('approve_modal.storage_class')}>
                                    <Input placeholder="e.g. rook-ceph-block" />
                                </Form.Item>
                                <Form.Item name="disk_gb" label={t('approve_modal.disk_gb')}>
                                    <UnitInputNumber min={1} max={500} unit="GB" />
                                </Form.Item>
                                <Form.Item name="enable_override" valuePropName="checked" label={t('approve_modal.enable_override')}>
                                    <Switch />
                                </Form.Item>
                                <Form.Item noStyle shouldUpdate={(prev, cur) => prev.enable_override !== cur.enable_override}>
                                    {({ getFieldValue }) =>
                                        getFieldValue('enable_override') ? (
                                            <Card size="small" style={{ marginBottom: 16, background: '#fafafa' }}>
                                                <Space direction="vertical" style={{ width: '100%' }}>
                                                    <Space style={{ width: '100%' }}>
                                                        <Form.Item
                                                            name="cpu_request"
                                                            label={t('approve_modal.cpu_request')}
                                                            style={{ marginBottom: 0, flex: 1 }}
                                                            dependencies={['cpu_limit']}
                                                            rules={[
                                                                ({ getFieldValue }) => ({
                                                                    validator(_, value) {
                                                                        const lim = getFieldValue('cpu_limit');
                                                                        if (payload?.dedicated_cpu && value && lim && value !== lim) {
                                                                            return Promise.reject(new Error(t('approve_modal.dedicated_cpu_no_overcommit', 'Dedicated CPU requires request == limit')));
                                                                        }
                                                                        return Promise.resolve();
                                                                    }
                                                                })
                                                            ]}
                                                        >
                                                            <UnitInputNumber min={0.5} step={0.5} precision={1} unit={t('approve_modal.cores')} />
                                                        </Form.Item>
                                                        <Form.Item
                                                            name="cpu_limit"
                                                            label={t('approve_modal.cpu_limit')}
                                                            style={{ marginBottom: 0, flex: 1 }}
                                                            dependencies={['cpu_request']}
                                                            rules={[
                                                                ({ getFieldValue }) => ({
                                                                    validator(_, value) {
                                                                        const req = getFieldValue('cpu_request');
                                                                        if (payload?.dedicated_cpu && req && value && req !== value) {
                                                                            return Promise.reject(new Error(t('approve_modal.dedicated_cpu_no_overcommit', 'Dedicated CPU requires request == limit')));
                                                                        }
                                                                        return Promise.resolve();
                                                                    }
                                                                })
                                                            ]}
                                                        >
                                                            <UnitInputNumber min={0.5} step={0.5} precision={1} unit={t('approve_modal.cores')} />
                                                        </Form.Item>
                                                    </Space>
                                                    <Space style={{ width: '100%' }}>
                                                        <Form.Item name="memory_request_gi" label={t('approve_modal.memory_request')} style={{ marginBottom: 0, flex: 1 }}>
                                                            <UnitInputNumber min={0.5} step={0.5} precision={1} unit="Gi" />
                                                        </Form.Item>
                                                        <Form.Item name="memory_limit_gi" label={t('approve_modal.memory_limit')} style={{ marginBottom: 0, flex: 1 }}>
                                                            <UnitInputNumber min={0.5} step={0.5} precision={1} unit="Gi" />
                                                        </Form.Item>
                                                    </Space>
                                                </Space>
                                                <Form.Item noStyle shouldUpdate={(prev, cur) =>
                                                    prev.cpu_request !== cur.cpu_request || prev.cpu_limit !== cur.cpu_limit ||
                                                    prev.memory_request_gi !== cur.memory_request_gi || prev.memory_limit_gi !== cur.memory_limit_gi
                                                }>
                                                    {({ getFieldValue: gfv }) => {
                                                        const cpuReq = gfv('cpu_request');
                                                        const cpuLim = gfv('cpu_limit');
                                                        const memReq = gfv('memory_request_gi');
                                                        const memLim = gfv('memory_limit_gi');
                                                        const isOvercommit = (cpuReq && cpuLim && cpuReq !== cpuLim) ||
                                                            (memReq && memLim && memReq !== memLim);
                                                        if (!isOvercommit) return null;
                                                        return (
                                                            <div style={{
                                                                padding: '8px 12px',
                                                                marginTop: 8,
                                                                background: '#fffbe6',
                                                                border: '1px solid #ffe58f',
                                                                borderRadius: 6,
                                                            }}>
                                                                <Space>
                                                                    <ExclamationCircleOutlined style={{ color: '#faad14' }} />
                                                                    <Text type="warning">{t('approve_modal.overcommit_warning')}</Text>
                                                                </Space>
                                                            </div>
                                                        );
                                                    }}
                                                </Form.Item>
                                            </Card>
                                        ) : null
                                    }
                                </Form.Item>
                            </>
                        );
                    })() : null}
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
