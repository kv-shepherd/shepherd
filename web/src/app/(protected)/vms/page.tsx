'use client';

/**
 * VMs page — lifecycle management with VM Request Wizard.
 *
 * OpenAPI: GET /vms, POST /vms/request, POST /vms/{vm_id}/start|stop|restart, DELETE /vms/{vm_id}
 * ADR-0017: User does NOT provide ClusterID; Namespace immutable after submission
 * ADR-0015: VM → Service → System hierarchy
 *
 * Features:
 * - VM list with status badges and power actions
 * - VM Request Wizard (multi-step: Service → Template → Size → Config → Confirm)
 * - Start/Stop/Restart actions via POST
 * - Delete with tiered confirmation (Stage 5.D)
 */
import { useState, useMemo } from 'react';
import {
    Table,
    Button,
    Space,
    Typography,
    Tag,
    Modal,
    Steps,
    Form,
    Input,
    Select,
    Card,
    Descriptions,
    message,
    Popconfirm,
    Alert,
    Divider,
    Badge,
    Tooltip,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    PlusOutlined,
    ReloadOutlined,
    DesktopOutlined,
    PlayCircleOutlined,
    PauseCircleOutlined,
    RedoOutlined,
    DeleteOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { useApiGet, useApiMutation, useApiAction } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type VM = components['schemas']['VM'];
type VMList = components['schemas']['VMList'];
type VMCreateRequest = components['schemas']['VMCreateRequest'];
type SystemList = components['schemas']['SystemList'];
type ServiceList = components['schemas']['ServiceList'];
type TemplateList = components['schemas']['TemplateList'];
type InstanceSizeList = components['schemas']['InstanceSizeList'];
type Template = components['schemas']['Template'];
type InstanceSize = components['schemas']['InstanceSize'];
type ApprovalTicketResponse = components['schemas']['ApprovalTicketResponse'];
type DeleteVMResponse = components['schemas']['DeleteVMResponse'];

const VM_STATUS_MAP: Record<string, { color: string; badge: 'success' | 'processing' | 'error' | 'warning' | 'default' }> = {
    CREATING: { color: 'cyan', badge: 'processing' },
    RUNNING: { color: 'green', badge: 'success' },
    STOPPING: { color: 'orange', badge: 'warning' },
    STOPPED: { color: 'default', badge: 'default' },
    DELETING: { color: 'orange', badge: 'warning' },
    FAILED: { color: 'red', badge: 'error' },
    PENDING: { color: 'gold', badge: 'warning' },
    MIGRATING: { color: 'blue', badge: 'processing' },
    PAUSED: { color: 'purple', badge: 'warning' },
    UNKNOWN: { color: 'default', badge: 'default' },
};

const formatMemory = (memoryMb: number): string => {
    if (!Number.isFinite(memoryMb) || memoryMb <= 0) return '0 MB';
    if (memoryMb % 1024 === 0) return `${memoryMb / 1024} Gi`;
    return `${memoryMb} MB`;
};

const capabilityTags = (size: InstanceSize, t: (k: string) => string) => {
    const tags: React.ReactNode[] = [];
    if (size.requires_gpu) tags.push(<Tag key="gpu" color="volcano">{t('capability.gpu')}</Tag>);
    if (size.requires_sriov) tags.push(<Tag key="sriov" color="purple">{t('capability.sriov')}</Tag>);
    if (size.requires_hugepages) {
        const hpLabel = size.hugepages_size ? `${t('capability.hugepages')}: ${size.hugepages_size}` : t('capability.hugepages');
        tags.push(<Tag key="hugepages" color="gold">{hpLabel}</Tag>);
    }
    if (size.dedicated_cpu) tags.push(<Tag key="dedicated" color="blue">{t('capability.dedicated_cpu')}</Tag>);
    return tags;
};

export default function VMsPage() {
    const { t } = useTranslation(['vm', 'common']);
    const [messageApi, contextHolder] = message.useMessage();
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [wizardOpen, setWizardOpen] = useState(false);
    const [wizardStep, setWizardStep] = useState(0);
    const [form] = Form.useForm();
    const [selectedSystemId, setSelectedSystemId] = useState<string>('');

    // Watch form fields for wizard (replaces getFieldValue in render/useMemo which causes "not connected" warning)
    const selectedTemplateId = Form.useWatch('template_id', form);
    const selectedSizeId = Form.useWatch('instance_size_id', form);
    const namespaceValue = Form.useWatch('namespace', form);
    const reasonValue = Form.useWatch('reason', form);
    const serviceIdValue = Form.useWatch('service_id', form);

    // Data fetches
    const { data: vmData, isLoading, refetch } = useApiGet<VMList>(
        ['vms', page, pageSize],
        () => api.GET('/vms', { params: { query: { page, per_page: pageSize } } })
    );

    const { data: systemsData } = useApiGet<SystemList>(
        ['systems', 'vm-wizard'],
        () => api.GET('/systems', { params: { query: { per_page: 100 } } }),
        { enabled: wizardOpen }
    );

    const { data: servicesData } = useApiGet<ServiceList>(
        ['services', selectedSystemId, 'vm-wizard'],
        () => api.GET('/systems/{system_id}/services', {
            params: { path: { system_id: selectedSystemId }, query: { per_page: 100 } },
        }),
        { enabled: wizardOpen && !!selectedSystemId }
    );

    const { data: templatesData } = useApiGet<TemplateList>(
        ['templates'],
        () => api.GET('/templates'),
        { enabled: wizardOpen }
    );

    const { data: sizesData } = useApiGet<InstanceSizeList>(
        ['instance-sizes'],
        () => api.GET('/instance-sizes'),
        { enabled: wizardOpen }
    );

    // Mutations
    const createVMRequest = useApiMutation<VMCreateRequest, ApprovalTicketResponse>(
        (req) => api.POST('/vms/request', { body: req }),
        {
            invalidateKeys: [['vms'], ['approvals']],
            onSuccess: () => {
                messageApi.success(t('request_submitted'));
                setWizardOpen(false);
                setWizardStep(0);
                form.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const startVM = useApiAction<string>(
        (vmId) => api.POST('/vms/{vm_id}/start', { params: { path: { vm_id: vmId } } }),
        { invalidateKeys: [['vms']], onSuccess: () => messageApi.success(t('common:message.success')) }
    );

    const stopVM = useApiAction<string>(
        (vmId) => api.POST('/vms/{vm_id}/stop', { params: { path: { vm_id: vmId } } }),
        { invalidateKeys: [['vms']], onSuccess: () => messageApi.success(t('common:message.success')) }
    );

    const restartVM = useApiAction<string>(
        (vmId) => api.POST('/vms/{vm_id}/restart', { params: { path: { vm_id: vmId } } }),
        { invalidateKeys: [['vms']], onSuccess: () => messageApi.success(t('common:message.success')) }
    );

    const deleteVM = useApiMutation<{ vmId: string; vmName: string }, DeleteVMResponse>(
        ({ vmId, vmName }) => api.DELETE('/vms/{vm_id}', {
            params: {
                path: { vm_id: vmId },
                // Keep both params for test/prod tiered confirmation compatibility.
                query: { confirm: true, confirm_name: vmName },
            },
        }),
        {
            invalidateKeys: [['vms'], ['approvals']],
            onSuccess: (resp) => messageApi.success(t('delete_request_submitted', { ticket_id: resp.ticket_id })),
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    // Table columns
    const columns: ColumnsType<VM> = [
        {
            title: t('field.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string) => (
                <Space>
                    <DesktopOutlined style={{ color: '#531dab' }} />
                    <Text strong>{name}</Text>
                </Space>
            ),
        },
        {
            title: t('common:table.status'),
            dataIndex: 'status',
            key: 'status',
            width: 130,
            render: (status: VM['status']) => {
                const map = VM_STATUS_MAP[status] ?? VM_STATUS_MAP.UNKNOWN;
                return (
                    <Badge status={map.badge} text={<Tag color={map.color}>{t(`status.${status}`)}</Tag>} />
                );
            },
        },
        {
            title: t('field.namespace'),
            dataIndex: 'namespace',
            key: 'namespace',
            width: 150,
            render: (ns: string) => <Tag>{ns}</Tag>,
        },
        {
            title: t('field.hostname'),
            dataIndex: 'hostname',
            key: 'hostname',
            width: 180,
            render: (h: string) => <Text type="secondary">{h || '—'}</Text>,
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
            width: 200,
            render: (_, record) => {
                const isRunning = record.status === 'RUNNING';
                const isStopped = record.status === 'STOPPED';
                const canDelete = isStopped || record.status === 'FAILED';

                return (
                    <Space size={4}>
                        <Tooltip title={t('action.start')}>
                            <Button
                                type="text"
                                size="small"
                                icon={<PlayCircleOutlined />}
                                disabled={!isStopped}
                                onClick={() => startVM.mutate(record.id)}
                                style={{ color: isStopped ? '#52c41a' : undefined }}
                            />
                        </Tooltip>
                        <Tooltip title={t('action.stop')}>
                            <Button
                                type="text"
                                size="small"
                                icon={<PauseCircleOutlined />}
                                disabled={!isRunning}
                                onClick={() => stopVM.mutate(record.id)}
                                style={{ color: isRunning ? '#faad14' : undefined }}
                            />
                        </Tooltip>
                        <Tooltip title={t('action.restart')}>
                            <Button
                                type="text"
                                size="small"
                                icon={<RedoOutlined />}
                                disabled={!isRunning}
                                onClick={() => restartVM.mutate(record.id)}
                            />
                        </Tooltip>
                        <Popconfirm
                            title={t('action.delete_confirm')}
                            description={t('action.delete_confirm_name', { name: record.name })}
                            onConfirm={() => deleteVM.mutate({ vmId: record.id, vmName: record.name })}
                            okText={t('common:button.confirm')}
                            cancelText={t('common:button.cancel')}
                        >
                            <Button type="text" size="small" danger icon={<DeleteOutlined />} disabled={!canDelete} />
                        </Popconfirm>
                    </Space>
                );
            },
        },
    ];

    // Wizard helpers
    const selectedTemplate = useMemo(() => {
        return templatesData?.items?.find((t: Template) => t.id === selectedTemplateId);
    }, [selectedTemplateId, templatesData]);

    const selectedSize = useMemo(() => {
        return sizesData?.items?.find((s: InstanceSize) => s.id === selectedSizeId);
    }, [selectedSizeId, sizesData]);

    const wizardSteps = [
        { title: t('wizard.step.service') },
        { title: t('wizard.step.template') },
        { title: t('wizard.step.size') },
        { title: t('wizard.step.config') },
        { title: t('wizard.step.confirm') },
    ];

    const handleWizardNext = async () => {
        try {
            // Validate fields for current step
            const fieldsByStep: string[][] = [
                ['service_id'],
                ['template_id'],
                ['instance_size_id'],
                ['namespace', 'reason'],
                [],
            ];
            if (fieldsByStep[wizardStep].length > 0) {
                await form.validateFields(fieldsByStep[wizardStep]);
            }
            setWizardStep((s) => s + 1);
        } catch {
            // validation errors handled by Ant Form
        }
    };

    const handleWizardSubmit = () => {
        const values = form.getFieldsValue();
        createVMRequest.mutate(values);
    };

    const renderWizardStep = () => {
        switch (wizardStep) {
            case 0: // Select Service
                return (
                    <>
                        <Form.Item label={t('wizard.select_system')} style={{ marginBottom: 16 }}>
                            <Select
                                placeholder={t('wizard.select_system')}
                                value={selectedSystemId || undefined}
                                onChange={(val) => {
                                    setSelectedSystemId(val);
                                    setTimeout(() => form.setFieldValue('service_id', undefined), 0);
                                }}
                                options={systemsData?.items?.map((s) => ({
                                    label: s.name,
                                    value: s.id,
                                }))}
                                style={{ width: '100%' }}
                            />
                        </Form.Item>
                        <Form.Item
                            name="service_id"
                            label={t('wizard.select_service')}
                            rules={[{ required: true, message: 'Please select a service' }]}
                        >
                            <Select
                                placeholder={t('wizard.select_service')}
                                disabled={!selectedSystemId}
                                options={servicesData?.items?.map((s) => ({
                                    label: s.name,
                                    value: s.id,
                                }))}
                                style={{ width: '100%' }}
                            />
                        </Form.Item>
                    </>
                );

            case 1: // Select Template
                return (
                    <Form.Item
                        name="template_id"
                        label={t('wizard.select_template')}
                        rules={[{ required: true, message: 'Please select a template' }]}
                    >
                        <Select
                            placeholder={t('wizard.select_template')}
                            options={templatesData?.items
                                ?.filter((t: Template) => t.enabled !== false)
                                .map((t: Template) => ({
                                    label: (
                                        <Space>
                                            <Text strong>{t.display_name || t.name}</Text>
                                            {t.os_family && <Tag color="blue">{t.os_family} {t.os_version}</Tag>}
                                        </Space>
                                    ),
                                    value: t.id,
                                }))}
                            style={{ width: '100%' }}
                        />
                    </Form.Item>
                );

            case 2: // Select Instance Size
                return (
                    <>
                        <Form.Item
                            name="instance_size_id"
                            label={t('wizard.select_size')}
                            rules={[{ required: true, message: 'Please select an instance size' }]}
                        >
                            <Select
                                placeholder={t('wizard.select_size')}
                                options={sizesData?.items
                                    ?.filter((s: InstanceSize) => s.enabled !== false)
                                    .map((s: InstanceSize) => ({
                                        label: (
                                            <Space direction="vertical" size={0}>
                                                <Space size={6}>
                                                    <Text strong>{s.display_name || s.name}</Text>
                                                    <Text type="secondary">{s.cpu_cores} vCPU · {formatMemory(s.memory_mb)}</Text>
                                                    {s.disk_gb && <Text type="secondary">· {s.disk_gb} GB</Text>}
                                                </Space>
                                                {capabilityTags(s, t).length > 0 && (
                                                    <Space size={4} wrap>
                                                        {capabilityTags(s, t)}
                                                    </Space>
                                                )}
                                            </Space>
                                        ),
                                        value: s.id,
                                    }))}
                                style={{ width: '100%' }}
                            />
                        </Form.Item>
                        {selectedSize && capabilityTags(selectedSize, t).length > 0 && (
                            <Alert
                                type={selectedSize.requires_gpu ? 'warning' : 'info'}
                                showIcon
                                message={t('wizard.size_capability_notice')}
                                description={<Space wrap>{capabilityTags(selectedSize, t)}</Space>}
                            />
                        )}
                    </>
                );

            case 3: // Configuration
                return (
                    <>
                        <Form.Item
                            name="namespace"
                            label={t('wizard.namespace')}
                            rules={[{ required: true, message: 'Namespace is required' }]}
                            extra={t('wizard.namespace_hint')}
                        >
                            <Input placeholder="e.g. production, staging" />
                        </Form.Item>
                        <Form.Item
                            name="reason"
                            label={t('wizard.reason')}
                            rules={[{ required: true, message: 'Request reason is required' }]}
                        >
                            <Input.TextArea
                                rows={4}
                                placeholder={t('wizard.reason_placeholder')}
                            />
                        </Form.Item>
                    </>
                );

            case 4: // Confirm
                return (
                    <div>
                        <Alert
                            type="info"
                            message={t('wizard.confirm_note')}
                            style={{ marginBottom: 16 }}
                            showIcon
                        />
                        <Descriptions bordered column={1} size="small">
                            <Descriptions.Item label={t('wizard.confirm_service')}>
                                {servicesData?.items?.find((s) => s.id === serviceIdValue)?.name ?? '—'}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_template')}>
                                {selectedTemplate?.display_name || selectedTemplate?.name || '—'}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_size')}>
                                {selectedSize ? `${selectedSize.display_name || selectedSize.name} (${selectedSize.cpu_cores} vCPU · ${formatMemory(selectedSize.memory_mb)})` : '—'}
                                {selectedSize && capabilityTags(selectedSize, t).length > 0 && (
                                    <div style={{ marginTop: 8 }}>
                                        <Space wrap>{capabilityTags(selectedSize, t)}</Space>
                                    </div>
                                )}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_namespace')}>
                                <Tag>{namespaceValue}</Tag>
                            </Descriptions.Item>
                            <Descriptions.Item label={t('wizard.confirm_reason')}>
                                {reasonValue}
                            </Descriptions.Item>
                        </Descriptions>
                    </div>
                );

            default:
                return null;
        }
    };

    return (
        <div>
            {contextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('title')}</Title>
                    <Text type="secondary">{t('subtitle')}</Text>
                </div>
                <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        onClick={() => {
                            setWizardOpen(true);
                            setWizardStep(0);
                            form.resetFields();
                            setSelectedSystemId('');
                        }}
                    >
                        {t('create_request')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<VM>
                    columns={columns}
                    dataSource={vmData?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={{
                        current: page,
                        pageSize,
                        total: vmData?.pagination?.total ?? 0,
                        showTotal: (total) => t('common:table.total', { total }),
                        onChange: (p, ps) => { setPage(p); setPageSize(ps); },
                    }}
                    size="middle"
                />
            </Card>

            {/* VM Request Wizard */}
            <Modal
                title={t('wizard.title')}
                open={wizardOpen}
                onCancel={() => { setWizardOpen(false); setWizardStep(0); form.resetFields(); }}
                width={720}
                footer={
                    <Space>
                        {wizardStep > 0 && (
                            <Button onClick={() => setWizardStep((s) => s - 1)}>
                                {t('common:button.prev')}
                            </Button>
                        )}
                        {wizardStep < wizardSteps.length - 1 ? (
                            <Button type="primary" onClick={handleWizardNext}>
                                {t('common:button.next')}
                            </Button>
                        ) : (
                            <Button
                                type="primary"
                                onClick={handleWizardSubmit}
                                loading={createVMRequest.isPending}
                            >
                                {t('common:button.submit')}
                            </Button>
                        )}
                    </Space>
                }
                forceRender
            >
                <Steps
                    current={wizardStep}
                    items={wizardSteps}
                    size="small"
                    style={{ marginBottom: 24 }}
                />
                <Divider />
                <Form form={form} layout="vertical" name="vm-request-wizard">
                    {renderWizardStep()}
                </Form>
            </Modal>
        </div>
    );
}
