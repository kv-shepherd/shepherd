'use client';

import {
    Button,
    Card,
    Form,
    Input,
    InputNumber,
    Modal,
    Popconfirm,
    Select,
    Space,
    Switch,
    Table,
    Tag,
    Tooltip,
    Typography,
} from 'antd';
import {
    DeleteOutlined,
    EditOutlined,
    LinkOutlined,
    PlusOutlined,
    ReloadOutlined,
    SafetyOutlined,
    SyncOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { useAdminAuthProvidersController } from '../hooks/useAdminAuthProvidersController';
import { type AuthProvider, type IdPGroupMapping } from '../types';

const { Title, Text } = Typography;

export function AdminAuthProvidersContent() {
    const { t } = useTranslation(['admin', 'common']);
    const providers = useAdminAuthProvidersController({ t });
    const scopeOptions = useMemo(
        () => [
            { value: 'global', label: t('rbac.scope.global') },
            { value: 'system', label: t('rbac.scope.system') },
            { value: 'service', label: t('rbac.scope.service') },
            { value: 'vm', label: t('rbac.scope.vm') },
        ],
        [t]
    );
    const environmentOptions = useMemo(
        () => [
            { value: 'test', label: t('authProviders.env.test') },
            { value: 'prod', label: t('authProviders.env.prod') },
        ],
        [t]
    );

    const columns: ColumnsType<AuthProvider> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string, record: AuthProvider) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{name}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>{record.id}</Text>
                </Space>
            ),
        },
        {
            title: t('authProviders.type'),
            dataIndex: 'auth_type',
            key: 'auth_type',
            width: 130,
            render: (authType: string) => (
                <Tag color="processing">
                    {providers.providerTypeLabelByKey[authType] ?? authType}
                </Tag>
            ),
        },
        {
            title: t('common:table.status'),
            dataIndex: 'enabled',
            key: 'enabled',
            width: 120,
            render: (enabled: boolean) => (
                <Tag color={enabled ? 'green' : 'default'}>
                    {enabled ? t('users.status.enabled') : t('users.status.disabled')}
                </Tag>
            ),
        },
        {
            title: t('authProviders.sort_order'),
            dataIndex: 'sort_order',
            key: 'sort_order',
            width: 120,
            render: (sortOrder?: number) => sortOrder ?? 0,
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'updated_at',
            key: 'updated_at',
            width: 180,
            render: (updatedAt?: string) => updatedAt ? dayjs(updatedAt).format('YYYY-MM-DD HH:mm') : '—',
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 240,
            render: (_, record: AuthProvider) => (
                <Space>
                    <Tooltip title={t('authProviders.test_connection')}>
                        <Button
                            type="text"
                            size="small"
                            data-testid={`auth-provider-action-test-${record.id}`}
                            icon={<LinkOutlined />}
                            loading={providers.testingProviderId === record.id && providers.testConnectionPending}
                            onClick={() => providers.testConnection(record)}
                        />
                    </Tooltip>
                    <Tooltip title={t('authProviders.group_mappings')}>
                        <Button
                            type="text"
                            size="small"
                            data-testid={`auth-provider-action-mapping-${record.id}`}
                            icon={<SafetyOutlined />}
                            onClick={() => providers.openMappingModal(record)}
                        />
                    </Tooltip>
                    <Tooltip title={t('common:button.edit')}>
                        <Button
                            type="text"
                            size="small"
                            data-testid={`auth-provider-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            onClick={() => providers.openEditModal(record)}
                        />
                    </Tooltip>
                    <Tooltip title={t('common:button.delete')}>
                        <Button
                            type="text"
                            size="small"
                            data-testid={`auth-provider-action-delete-${record.id}`}
                            danger
                            icon={<DeleteOutlined />}
                            onClick={() => providers.openDeleteModal(record)}
                        />
                    </Tooltip>
                </Space>
            ),
        },
    ];

    const mappingColumns: ColumnsType<IdPGroupMapping> = [
        {
            title: t('authProviders.mapping.group'),
            key: 'group',
            render: (_, record) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{record.group_name || record.external_group_id}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>{record.external_group_id}</Text>
                </Space>
            ),
        },
        {
            title: t('authProviders.mapping.role'),
            key: 'role',
            render: (_, record) => record.role_name || record.role_id,
        },
        {
            title: t('authProviders.mapping.envs'),
            key: 'allowed_environments',
            render: (_, record) => (
                <Space size={4}>
                    {(record.allowed_environments ?? []).map((env) => (
                        <Tag key={`${record.id}-${env}`} color={env === 'prod' ? 'red' : 'blue'}>
                            {t(`authProviders.env.${env}`, { defaultValue: env })}
                        </Tag>
                    ))}
                </Space>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 120,
            render: (_, record) => (
                <Space>
                    <Button
                        type="text"
                        size="small"
                        icon={<EditOutlined />}
                        onClick={() => providers.openEditMappingModal(record)}
                    />
                    <Popconfirm
                        title={t('authProviders.mapping.delete_confirm')}
                        onConfirm={() => providers.deleteMapping(record)}
                    >
                        <Button type="text" size="small" danger icon={<DeleteOutlined />} />
                    </Popconfirm>
                </Space>
            ),
        },
    ];

    return (
        <div>
            {providers.messageContextHolder}
            <div style={{ marginBottom: 24 }}>
                <Title level={4} style={{ margin: 0 }}>{t('authProviders.title')}</Title>
                <Text type="secondary">{t('authProviders.subtitle')}</Text>
            </div>

            <Card style={{ borderRadius: 12 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Text>{t('authProviders.config_help')}</Text>
                    <Space>
                        <Button icon={<ReloadOutlined />} onClick={() => providers.refetchProviders()}>
                            {t('common:button.refresh')}
                        </Button>
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            data-testid="auth-provider-create-button"
                            onClick={providers.openCreateModal}
                        >
                            {t('authProviders.add')}
                        </Button>
                    </Space>
                </Space>

                <Table<AuthProvider>
                    style={{ marginTop: 16 }}
                    rowKey="id"
                    columns={columns}
                    dataSource={providers.providers}
                    loading={providers.providersLoading}
                    pagination={false}
                />
            </Card>

            <Modal
                title={t('authProviders.add_title')}
                open={providers.createOpen}
                onOk={() => {
                    void providers.submitCreate();
                }}
                onCancel={providers.closeCreateModal}
                confirmLoading={providers.createPending}
                destroyOnHidden={true}
            >
                <Form form={providers.createForm} layout="vertical" preserve={false}>
                    <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="auth_type" label={t('authProviders.type')} rules={[{ required: true }]}>
                        <Select
                            options={providers.providerTypeOptions}
                            loading={providers.providerTypesLoading}
                            showSearch={true}
                            optionFilterProp="label"
                        />
                    </Form.Item>
                    <Form.Item name="sort_order" label={t('authProviders.sort_order')}>
                        <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('common:table.status')} valuePropName="checked" initialValue={true}>
                        <Switch />
                    </Form.Item>
                    <Form.Item name="config_text" label={t('authProviders.config')}>
                        <Input.TextArea rows={12} style={{ fontFamily: 'monospace' }} />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('authProviders.edit_title', { name: providers.editingProvider?.name || '' })}
                open={providers.editOpen}
                onOk={() => {
                    void providers.submitEdit();
                }}
                onCancel={providers.closeEditModal}
                confirmLoading={providers.updatePending}
                destroyOnHidden={true}
            >
                <Form form={providers.editForm} layout="vertical" preserve={false}>
                    <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="sort_order" label={t('authProviders.sort_order')}>
                        <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('common:table.status')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item name="config_text" label={t('authProviders.config')}>
                        <Input.TextArea rows={12} style={{ fontFamily: 'monospace' }} />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('common:button.delete')}
                open={providers.deleteOpen}
                onOk={providers.submitDelete}
                onCancel={providers.closeDeleteModal}
                confirmLoading={providers.deletePending}
                okButtonProps={{ danger: true }}
            >
                <Text>{t('authProviders.delete_confirm', { name: providers.deletingProvider?.name || '' })}</Text>
            </Modal>

            <Modal
                title={t('authProviders.mapping.modal_title', { name: providers.mappingProvider?.name || '' })}
                open={providers.mappingOpen}
                onCancel={providers.closeMappingModal}
                footer={null}
                width={980}
                destroyOnHidden={true}
            >
                <Space direction="vertical" size={20} style={{ width: '100%' }}>
                    <Card size="small" title={t('authProviders.sample.title')} extra={
                        <Button
                            icon={<SyncOutlined />}
                            onClick={() => providers.testConnection(providers.mappingProvider as AuthProvider)}
                            loading={providers.testConnectionPending}
                            disabled={!providers.mappingProvider}
                        >
                            {t('authProviders.test_connection')}
                        </Button>
                    }>
                        <Table
                            rowKey="field"
                            size="small"
                            pagination={false}
                            loading={providers.sampleLoading}
                            dataSource={providers.sampleFields}
                            columns={[
                                { title: t('authProviders.sample.field'), dataIndex: 'field', key: 'field' },
                                { title: t('authProviders.sample.value_type'), dataIndex: 'value_type', key: 'value_type', width: 120 },
                                { title: t('authProviders.sample.unique_count'), dataIndex: 'unique_count', key: 'unique_count', width: 120 },
                                {
                                    title: t('authProviders.sample.sample'),
                                    key: 'sample',
                                    render: (_, record) => (record.sample ?? []).join(', '),
                                },
                            ]}
                        />
                    </Card>

                    <Card size="small" title={t('authProviders.sync.title')}>
                        <Form form={providers.syncForm} layout="vertical">
                            <Form.Item
                                name="source_field"
                                label={t('authProviders.sync.source_field')}
                                rules={[{ required: true }]}
                            >
                                <Input />
                            </Form.Item>
                            <Form.Item
                                name="groups_text"
                                label={t('authProviders.sync.groups')}
                                rules={[{ required: true }]}
                            >
                                <Input.TextArea rows={4} />
                            </Form.Item>
                            <Button
                                type="primary"
                                icon={<SyncOutlined />}
                                loading={providers.syncGroupsPending}
                                onClick={() => {
                                    void providers.submitSyncGroups();
                                }}
                            >
                                {t('authProviders.sync.submit')}
                            </Button>
                        </Form>
                    </Card>

                    <Card size="small" title={t('authProviders.mapping.title')}>
                        <Form form={providers.mappingForm} layout="vertical">
                            <Space style={{ width: '100%' }} align="start" wrap>
                                <Form.Item
                                    name="external_group_id"
                                    label={t('authProviders.mapping.group')}
                                    rules={[{ required: true }]}
                                    style={{ minWidth: 240 }}
                                >
                                    <Input />
                                </Form.Item>
                                <Form.Item
                                    name="group_name"
                                    label={t('authProviders.mapping.group_name')}
                                    style={{ minWidth: 240 }}
                                >
                                    <Input />
                                </Form.Item>
                                <Form.Item
                                    name="role_id"
                                    label={t('authProviders.mapping.role')}
                                    rules={[{ required: true }]}
                                    style={{ minWidth: 220 }}
                                >
                                    <Select options={providers.roleOptions} />
                                </Form.Item>
                                <Form.Item name="scope_type" label={t('authProviders.mapping.scope_type')} style={{ minWidth: 150 }}>
                                    <Select options={scopeOptions} />
                                </Form.Item>
                                <Form.Item name="scope_id" label={t('authProviders.mapping.scope_id')} style={{ minWidth: 180 }}>
                                    <Input />
                                </Form.Item>
                                <Form.Item
                                    name="allowed_environments"
                                    label={t('authProviders.mapping.envs')}
                                    style={{ minWidth: 220 }}
                                >
                                    <Select mode="multiple" options={environmentOptions} />
                                </Form.Item>
                            </Space>
                            <Button
                                type="primary"
                                icon={<PlusOutlined />}
                                loading={providers.createMappingPending}
                                onClick={() => {
                                    void providers.submitCreateMapping();
                                }}
                            >
                                {t('authProviders.mapping.add')}
                            </Button>
                        </Form>

                        <Table<IdPGroupMapping>
                            style={{ marginTop: 16 }}
                            rowKey="id"
                            size="small"
                            columns={mappingColumns}
                            dataSource={providers.mappings}
                            loading={providers.mappingsLoading}
                            pagination={false}
                        />
                    </Card>
                </Space>
            </Modal>

            <Modal
                title={t('authProviders.mapping.edit_title')}
                open={providers.editMappingOpen}
                onCancel={providers.closeEditMappingModal}
                onOk={() => {
                    void providers.submitEditMapping();
                }}
                confirmLoading={providers.updateMappingPending}
                destroyOnHidden={true}
            >
                <Form form={providers.mappingEditForm} layout="vertical">
                    <Form.Item name="role_id" label={t('authProviders.mapping.role')} rules={[{ required: true }]}>
                        <Select options={providers.roleOptions} />
                    </Form.Item>
                    <Form.Item name="scope_type" label={t('authProviders.mapping.scope_type')}>
                        <Select options={scopeOptions} />
                    </Form.Item>
                    <Form.Item name="scope_id" label={t('authProviders.mapping.scope_id')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="allowed_environments" label={t('authProviders.mapping.envs')}>
                        <Select mode="multiple" options={environmentOptions} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
