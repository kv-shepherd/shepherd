'use client';

import {
    Button,
    Card,
    Form,
    Input,
    Modal,
    Select,
    Space,
    Switch,
    Table,
    Tag,
    Tooltip,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    DeleteOutlined,
    EditOutlined,
    ExclamationCircleOutlined,
    GlobalOutlined,
    PlusOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { useAdminNamespacesController } from '../hooks/useAdminNamespacesController';
import { ENV_MAP, ENV_OPTIONS, type NamespaceRegistry } from '../types';

const { Title, Text, Paragraph } = Typography;

export function AdminNamespacesContent() {
    const { t } = useTranslation(['admin', 'common']);
    const namespaces = useAdminNamespacesController({ t });

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
                            onClick={() => namespaces.openEditModal(record)}
                        />
                    </Tooltip>
                    <Tooltip title={t('common:button.delete')}>
                        <Button
                            type="text"
                            size="small"
                            danger
                            icon={<DeleteOutlined />}
                            onClick={() => namespaces.openDeleteModal(record)}
                        />
                    </Tooltip>
                </Space>
            ),
        },
    ];

    const handleCreate = () => {
        void namespaces.submitCreate();
    };

    const handleUpdate = () => {
        void namespaces.submitUpdate();
    };

    return (
        <div>
            {namespaces.messageContextHolder}
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
                        value={namespaces.envFilter || undefined}
                        onChange={namespaces.changeEnvFilter}
                        options={ENV_OPTIONS.map((item) => ({ ...item }))}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => namespaces.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        onClick={namespaces.openCreateModal}
                    >
                        {t('namespaces.add')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<NamespaceRegistry>
                    columns={columns}
                    dataSource={namespaces.data?.items ?? []}
                    rowKey="id"
                    loading={namespaces.isLoading}
                    pagination={{
                        current: namespaces.page,
                        total: namespaces.data?.pagination?.total ?? 0,
                        pageSize: 20,
                        onChange: namespaces.setPage,
                        showTotal: (total) => t('common:table.total', { total }),
                    }}
                    size="middle"
                />
            </Card>

            <Modal
                title={t('namespaces.add')}
                open={namespaces.createOpen}
                onOk={handleCreate}
                onCancel={namespaces.closeCreateModal}
                confirmLoading={namespaces.createPending}
                forceRender
            >
                <Form form={namespaces.createForm} layout="vertical" name="create-namespace">
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
                        <Select options={ENV_OPTIONS.map((item) => ({ ...item }))} />
                    </Form.Item>
                    <Form.Item
                        name="description"
                        label={t('common:table.description')}
                    >
                        <Input.TextArea rows={3} placeholder={t('namespaces.desc_placeholder')} />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={`${t('common:button.edit')}: ${namespaces.editingNs?.name ?? ''}`}
                open={namespaces.editOpen}
                onOk={handleUpdate}
                onCancel={namespaces.closeEditModal}
                confirmLoading={namespaces.updatePending}
                forceRender
            >
                <Form form={namespaces.editForm} layout="vertical" name="edit-namespace">
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

            <Modal
                title={(
                    <Space>
                        <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />
                        {t('namespaces.delete_title')}
                    </Space>
                )}
                open={namespaces.deleteOpen}
                onOk={namespaces.submitDelete}
                onCancel={namespaces.closeDeleteModal}
                confirmLoading={namespaces.deletePending}
                okButtonProps={{
                    danger: true,
                    disabled: namespaces.deleteConfirmName !== namespaces.deletingNs?.name,
                }}
                okText={t('common:button.delete')}
            >
                <Paragraph>
                    {t('namespaces.delete_confirm', { name: namespaces.deletingNs?.name })}
                </Paragraph>
                <Paragraph type="secondary">
                    {t('namespaces.delete_type_name')}
                </Paragraph>
                <Input
                    value={namespaces.deleteConfirmName}
                    onChange={(e) => namespaces.setDeleteConfirmName(e.target.value)}
                    placeholder={namespaces.deletingNs?.name}
                    status={
                        namespaces.deleteConfirmName && namespaces.deleteConfirmName !== namespaces.deletingNs?.name
                            ? 'error'
                            : undefined
                    }
                />
            </Modal>
        </div>
    );
}
