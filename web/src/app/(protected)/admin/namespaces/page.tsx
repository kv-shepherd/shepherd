'use client';

/**
 * Namespace management — admin page with full CRUD.
 *
 * OpenAPI: GET/POST /admin/namespaces, GET/PUT/DELETE /admin/namespaces/{id}
 * master-flow Stage 3, Step 2: Namespace is a global logical entity (ADR-0017).
 * ADR-0015 §13: Delete requires confirm_name.
 */
import { useState } from 'react';
import {
    Table,
    Button,
    Space,
    Typography,
    Tag,
    message,
    Modal,
    Form,
    Input,
    Select,
    Switch,
    Card,
    Popconfirm,
    Tooltip,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    PlusOutlined,
    ReloadOutlined,
    GlobalOutlined,
    EditOutlined,
    DeleteOutlined,
    ExclamationCircleOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { useApiGet, useApiMutation, useApiAction } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text, Paragraph } = Typography;

type NamespaceRegistry = components['schemas']['NamespaceRegistry'];
type NamespaceRegistryList = components['schemas']['NamespaceRegistryList'];
type NamespaceCreateRequest = components['schemas']['NamespaceCreateRequest'];
type NamespaceUpdateRequest = components['schemas']['NamespaceUpdateRequest'];

const ENV_MAP: Record<string, { color: string; label: string }> = {
    test: { color: 'blue', label: 'Test' },
    prod: { color: 'red', label: 'Production' },
};

export default function NamespacesPage() {
    const { t } = useTranslation(['admin', 'common']);
    const [messageApi, contextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [editOpen, setEditOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [editingNs, setEditingNs] = useState<NamespaceRegistry | null>(null);
    const [deletingNs, setDeletingNs] = useState<NamespaceRegistry | null>(null);
    const [deleteConfirmName, setDeleteConfirmName] = useState('');
    const [envFilter, setEnvFilter] = useState<string>('');
    const [page, setPage] = useState(1);
    const [createForm] = Form.useForm<NamespaceCreateRequest>();
    const [editForm] = Form.useForm<NamespaceUpdateRequest>();

    // Fetch namespaces
    const { data, isLoading, refetch } = useApiGet<NamespaceRegistryList>(
        ['admin-namespaces', page, envFilter],
        () => api.GET('/admin/namespaces', {
            params: {
                query: {
                    page,
                    per_page: 20,
                    ...(envFilter ? { environment: envFilter as 'test' | 'prod' } : {}),
                },
            },
        })
    );

    // Create namespace
    const createMutation = useApiMutation<NamespaceCreateRequest, NamespaceRegistry>(
        (req) => api.POST('/admin/namespaces', { body: req }),
        {
            invalidateKeys: [['admin-namespaces']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setCreateOpen(false);
                createForm.resetFields();
            },
            onError: (err) => {
                if (err.code === 'NAMESPACE_NAME_EXISTS') {
                    messageApi.error(t('namespaces.error.name_exists'));
                } else {
                    messageApi.error(err.message || t('common:message.error'));
                }
            },
        }
    );

    // Update namespace
    const updateMutation = useApiMutation<
        { id: string; body: NamespaceUpdateRequest },
        NamespaceRegistry
    >(
        ({ id, body }) => api.PUT('/admin/namespaces/{namespace_id}', {
            params: { path: { namespace_id: id } },
            body,
        }),
        {
            invalidateKeys: [['admin-namespaces']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setEditOpen(false);
                setEditingNs(null);
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    // Delete namespace
    const deleteMutation = useApiAction<{ id: string; confirmName: string }>(
        ({ id, confirmName }) => api.DELETE('/admin/namespaces/{namespace_id}', {
            params: {
                path: { namespace_id: id },
                query: { confirm_name: confirmName },
            },
        }),
        {
            invalidateKeys: [['admin-namespaces']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setDeleteOpen(false);
                setDeletingNs(null);
                setDeleteConfirmName('');
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const columns: ColumnsType<NamespaceRegistry> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string, record: NamespaceRegistry) => (
                <Space>
                    <GlobalOutlined style={{ color: '#1677ff' }} />
                    <div>
                        <Text strong>{name}</Text>
                        {record.description && (
                            <>
                                <br />
                                <Text type="secondary" style={{ fontSize: 12 }}>
                                    {record.description}
                                </Text>
                            </>
                        )}
                    </div>
                </Space>
            ),
        },
        {
            title: t('namespaces.environment'),
            dataIndex: 'environment',
            key: 'environment',
            width: 130,
            render: (env: string) => {
                const config = ENV_MAP[env] ?? { color: 'default', label: env };
                return <Tag color={config.color}>{config.label}</Tag>;
            },
        },
        {
            title: t('namespaces.enabled'),
            dataIndex: 'enabled',
            key: 'enabled',
            width: 90,
            render: (enabled: boolean) => (
                <Tag color={enabled ? 'green' : 'default'}>
                    {enabled ? t('namespaces.enabled_yes') : t('namespaces.enabled_no')}
                </Tag>
            ),
        },
        {
            title: t('common:table.created_by'),
            dataIndex: 'created_by',
            key: 'created_by',
            width: 140,
            render: (actor: string) => <Text type="secondary">{actor || '—'}</Text>,
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
            width: 120,
            render: (_: unknown, record: NamespaceRegistry) => (
                <Space size="small">
                    <Tooltip title={t('common:button.edit')}>
                        <Button
                            type="text"
                            size="small"
                            icon={<EditOutlined />}
                            onClick={() => {
                                setEditingNs(record);
                                editForm.setFieldsValue({
                                    description: record.description ?? '',
                                    enabled: record.enabled ?? true,
                                });
                                setEditOpen(true);
                            }}
                        />
                    </Tooltip>
                    <Tooltip title={t('common:button.delete')}>
                        <Button
                            type="text"
                            size="small"
                            danger
                            icon={<DeleteOutlined />}
                            onClick={() => {
                                setDeletingNs(record);
                                setDeleteConfirmName('');
                                setDeleteOpen(true);
                            }}
                        />
                    </Tooltip>
                </Space>
            ),
        },
    ];

    const handleCreate = () => {
        createForm.validateFields().then((values) => {
            createMutation.mutate(values);
        });
    };

    const handleUpdate = () => {
        if (!editingNs) return;
        editForm.validateFields().then((values) => {
            updateMutation.mutate({ id: editingNs.id, body: values });
        });
    };

    const handleDelete = () => {
        if (!deletingNs) return;
        deleteMutation.mutate({ id: deletingNs.id, confirmName: deleteConfirmName });
    };

    return (
        <div>
            {contextHolder}
            <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 24,
            }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('namespaces.title')}</Title>
                    <Text type="secondary">{t('namespaces.subtitle')}</Text>
                </div>
                <Space>
                    <Select
                        placeholder={t('namespaces.filter_env')}
                        allowClear
                        style={{ width: 160 }}
                        value={envFilter || undefined}
                        onChange={(v) => { setEnvFilter(v ?? ''); setPage(1); }}
                        options={[
                            { value: 'test', label: 'Test' },
                            { value: 'prod', label: 'Production' },
                        ]}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        onClick={() => setCreateOpen(true)}
                    >
                        {t('namespaces.add')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<NamespaceRegistry>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={{
                        current: page,
                        total: data?.pagination?.total ?? 0,
                        pageSize: 20,
                        onChange: setPage,
                        showTotal: (total) => t('common:table.total', { total }),
                    }}
                    size="middle"
                />
            </Card>

            {/* Create Namespace Modal */}
            <Modal
                title={t('namespaces.add')}
                open={createOpen}
                onOk={handleCreate}
                onCancel={() => { setCreateOpen(false); createForm.resetFields(); }}
                confirmLoading={createMutation.isPending}
                forceRender
            >
                <Form form={createForm} layout="vertical" name="create-namespace">
                    <Form.Item
                        name="name"
                        label={t('common:table.name')}
                        rules={[
                            { required: true, message: t('namespaces.validation.name_required') },
                            { max: 63, message: t('namespaces.validation.name_max') },
                            {
                                pattern: /^[a-z][a-z0-9-]*$/,
                                message: t('namespaces.validation.name_format'),
                            },
                        ]}
                        extra={t('namespaces.name_hint')}
                    >
                        <Input placeholder="e.g. prod-shop, dev-analytics" />
                    </Form.Item>
                    <Form.Item
                        name="environment"
                        label={t('namespaces.environment')}
                        rules={[{ required: true, message: t('namespaces.validation.env_required') }]}
                        extra={t('namespaces.env_hint')}
                    >
                        <Select
                            options={[
                                { value: 'test', label: 'Test' },
                                { value: 'prod', label: 'Production' },
                            ]}
                        />
                    </Form.Item>
                    <Form.Item
                        name="description"
                        label={t('common:table.description')}
                    >
                        <Input.TextArea rows={3} placeholder={t('namespaces.desc_placeholder')} />
                    </Form.Item>
                </Form>
            </Modal>

            {/* Edit Namespace Modal */}
            <Modal
                title={`${t('common:button.edit')}: ${editingNs?.name ?? ''}`}
                open={editOpen}
                onOk={handleUpdate}
                onCancel={() => { setEditOpen(false); setEditingNs(null); }}
                confirmLoading={updateMutation.isPending}
                forceRender
            >
                <Form form={editForm} layout="vertical" name="edit-namespace">
                    <Paragraph type="secondary" style={{ marginBottom: 16 }}>
                        {t('namespaces.edit_note')}
                    </Paragraph>
                    <Form.Item
                        name="description"
                        label={t('common:table.description')}
                    >
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item
                        name="enabled"
                        label={t('namespaces.enabled')}
                        valuePropName="checked"
                    >
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            {/* Delete Namespace Modal (confirm_name gate, ADR-0015 §13) */}
            <Modal
                title={
                    <Space>
                        <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />
                        {t('namespaces.delete_title')}
                    </Space>
                }
                open={deleteOpen}
                onOk={handleDelete}
                onCancel={() => { setDeleteOpen(false); setDeletingNs(null); setDeleteConfirmName(''); }}
                confirmLoading={deleteMutation.isPending}
                okButtonProps={{
                    danger: true,
                    disabled: deleteConfirmName !== deletingNs?.name,
                }}
                okText={t('common:button.delete')}
            >
                <Paragraph>
                    {t('namespaces.delete_confirm', { name: deletingNs?.name })}
                </Paragraph>
                <Paragraph type="secondary">
                    {t('namespaces.delete_type_name')}
                </Paragraph>
                <Input
                    value={deleteConfirmName}
                    onChange={(e) => setDeleteConfirmName(e.target.value)}
                    placeholder={deletingNs?.name}
                    status={deleteConfirmName && deleteConfirmName !== deletingNs?.name ? 'error' : undefined}
                />
            </Modal>
        </div>
    );
}
