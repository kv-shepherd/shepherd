'use client';

import {
    Button,
    Card,
    Form,
    Input,
    Modal,
    Popconfirm,
    Select,
    Space,
    Switch,
    Table,
    Tag,
    Typography,
} from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { useAdminRbacController } from '../hooks/useAdminRbacController';
import {
    ENVIRONMENT_VALUES,
    RBAC_SCOPE_VALUES,
    type GlobalRoleBinding,
    type Permission,
    type Role,
} from '../types';

const { Title, Text } = Typography;

export function AdminRbacContent() {
    const { t } = useTranslation(['admin', 'common']);
    const rbac = useAdminRbacController({ t });

    const roleColumns: ColumnsType<Role> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string, role: Role) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{role.display_name || name}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>{name}</Text>
                </Space>
            ),
        },
        {
            title: t('common:table.description'),
            dataIndex: 'description',
            key: 'description',
            render: (description?: string) => description || '—',
        },
        {
            title: t('rbac.roles.permissions'),
            dataIndex: 'permissions',
            key: 'permissions',
            width: 420,
            render: (permissions: string[]) => (
                <Space wrap>
                    {(permissions || []).map((key) => (
                        <Tag key={key} color="processing">{key}</Tag>
                    ))}
                </Space>
            ),
        },
        {
            title: t('rbac.roles.built_in'),
            dataIndex: 'built_in',
            key: 'built_in',
            width: 100,
            render: (builtIn: boolean) => (
                <Tag color={builtIn ? 'gold' : 'default'}>
                    {builtIn ? t('rbac.boolean.yes') : t('rbac.boolean.no')}
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
                    {enabled ? t('common:status.active') : t('common:status.disabled')}
                </Tag>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 180,
            render: (_, role: Role) => (
                <Space>
                    <Button
                        size="small"
                        icon={<EditOutlined />}
                        disabled={role.built_in}
                        onClick={() => rbac.openEditRoleModal(role)}
                    >
                        {t('common:button.edit')}
                    </Button>
                    <Button
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        disabled={role.built_in}
                        onClick={() => rbac.openDeleteRoleModal(role)}
                    >
                        {t('common:button.delete')}
                    </Button>
                </Space>
            ),
        },
    ];

    const bindingColumns: ColumnsType<GlobalRoleBinding> = [
        {
            title: t('rbac.bindings.role'),
            dataIndex: 'role_name',
            key: 'role_name',
            render: (roleName: string, record: GlobalRoleBinding) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{roleName || record.role_id}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>{record.role_id}</Text>
                </Space>
            ),
        },
        {
            title: t('rbac.bindings.scope_type'),
            dataIndex: 'scope_type',
            key: 'scope_type',
            width: 130,
            render: (scopeType: string) => (
                <Tag>{t(`rbac.scope.${scopeType}`, { defaultValue: scopeType })}</Tag>
            ),
        },
        {
            title: t('rbac.bindings.scope_id'),
            dataIndex: 'scope_id',
            key: 'scope_id',
            render: (scopeID?: string) => scopeID || '—',
        },
        {
            title: t('rbac.bindings.allowed_envs'),
            dataIndex: 'allowed_environments',
            key: 'allowed_environments',
            width: 180,
            render: (envs?: Array<'test' | 'prod'>) => (
                <Space wrap>
                    {(envs || []).length > 0
                        ? (envs || []).map((env) => (
                            <Tag key={env}>{t(`rbac.env.${env}`, { defaultValue: env })}</Tag>
                        ))
                        : <Text type="secondary">—</Text>}
                </Space>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 120,
            render: (_, binding: GlobalRoleBinding) => (
                <Popconfirm
                    title={t('rbac.bindings.delete_confirm')}
                    onConfirm={() => rbac.deleteRoleBinding(binding.id)}
                    okText={t('common:button.confirm')}
                    cancelText={t('common:button.cancel')}
                >
                    <Button
                        danger
                        size="small"
                        loading={rbac.deleteBindingPending && rbac.deletingBindingId === binding.id}
                    >
                        {t('common:button.delete')}
                    </Button>
                </Popconfirm>
            ),
        },
    ];

    const permissionColumns: ColumnsType<Permission> = [
        {
            title: t('common:table.name'),
            dataIndex: 'key',
            key: 'key',
            render: (key: string) => <Text code>{key}</Text>,
        },
        {
            title: t('common:table.description'),
            dataIndex: 'description',
            key: 'description',
            render: (description?: string) => description || '—',
        },
    ];

    const roleOptions = useMemo(
        () => rbac.roles.map((role) => ({
            value: role.id,
            label: role.display_name || role.name,
        })),
        [rbac.roles]
    );
    const scopeOptions = useMemo(
        () => RBAC_SCOPE_VALUES.map((scope) => ({
            value: scope,
            label: t(`rbac.scope.${scope}`),
        })),
        [t]
    );
    const environmentOptions = useMemo(
        () => ENVIRONMENT_VALUES.map((env) => ({
            value: env,
            label: t(`rbac.env.${env}`),
        })),
        [t]
    );

    return (
        <div>
            {rbac.messageContextHolder}
            <div style={{ marginBottom: 24 }}>
                <Title level={4} style={{ margin: 0 }}>{t('rbac.title')}</Title>
                <Text type="secondary">{t('rbac.subtitle')}</Text>
            </div>

            <Card style={{ borderRadius: 12, marginBottom: 16 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0}>
                        <Text strong>{t('rbac.roles.title')}</Text>
                        <Text type="secondary">{t('rbac.roles.subtitle')}</Text>
                    </Space>
                    <Space>
                        <Button icon={<ReloadOutlined />} onClick={() => {
                            void rbac.refetchRoles();
                            void rbac.refetchPermissions();
                        }}>
                            {t('common:button.refresh')}
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} onClick={rbac.openCreateRoleModal}>
                            {t('rbac.roles.add')}
                        </Button>
                    </Space>
                </Space>

                <Table<Role>
                    style={{ marginTop: 16 }}
                    rowKey="id"
                    columns={roleColumns}
                    dataSource={rbac.roles}
                    loading={rbac.rolesLoading}
                    pagination={false}
                />
            </Card>

            <Card style={{ borderRadius: 12, marginBottom: 16 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0}>
                        <Text strong>{t('rbac.bindings.title')}</Text>
                        <Text type="secondary">{t('rbac.bindings.subtitle')}</Text>
                    </Space>
                    <Space>
                        <Button icon={<ReloadOutlined />} onClick={() => {
                            void rbac.refetchUsers();
                            if (rbac.selectedUserId) {
                                void rbac.refetchRoleBindings();
                            }
                        }}>
                            {t('common:button.refresh')}
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} onClick={rbac.openAddBindingModal}>
                            {t('rbac.bindings.add')}
                        </Button>
                    </Space>
                </Space>

                <Space align="center" style={{ marginTop: 16, marginBottom: 16 }}>
                    <SafetyCertificateOutlined />
                    <Text>{t('rbac.bindings.select_user')}</Text>
                    <Select
                        showSearch
                        optionFilterProp="label"
                        style={{ minWidth: 320 }}
                        value={rbac.selectedUserId || undefined}
                        loading={rbac.usersLoading}
                        placeholder={t('rbac.bindings.select_user_placeholder')}
                        onChange={(value) => rbac.setSelectedUserId(value)}
                        options={rbac.users.map((user) => ({
                            value: user.id,
                            label: `${user.username}${user.display_name ? ` (${user.display_name})` : ''}`,
                        }))}
                    />
                </Space>

                <Table<GlobalRoleBinding>
                    rowKey="id"
                    columns={bindingColumns}
                    dataSource={rbac.roleBindings}
                    loading={rbac.roleBindingsLoading}
                    locale={{
                        emptyText: rbac.selectedUserId
                            ? t('common:message.no_data')
                            : t('rbac.bindings.select_user_first'),
                    }}
                    pagination={false}
                />
            </Card>

            <Card style={{ borderRadius: 12 }}>
                <Space direction="vertical" size={0} style={{ marginBottom: 16 }}>
                    <Text strong>{t('rbac.permissions.title')}</Text>
                    <Text type="secondary">{t('rbac.permissions.subtitle')}</Text>
                </Space>
                <Table<Permission>
                    rowKey="key"
                    columns={permissionColumns}
                    dataSource={rbac.permissions}
                    loading={rbac.permissionsLoading}
                    pagination={false}
                />
            </Card>

            <Modal
                title={t('rbac.roles.add_title')}
                open={rbac.createRoleOpen}
                onOk={() => {
                    void rbac.submitCreateRole();
                }}
                onCancel={rbac.closeCreateRoleModal}
                confirmLoading={rbac.createRolePending}
                destroyOnHidden={true}
            >
                <Form form={rbac.roleCreateForm} layout="vertical" preserve={false}>
                    <Form.Item name="name" label={t('common:table.name')} rules={[{ required: true }]}> 
                        <Input />
                    </Form.Item>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="description" label={t('common:table.description')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="permissions" label={t('rbac.roles.permissions')} rules={[{ required: true }]}> 
                        <Select mode="multiple" options={rbac.permissionOptions} optionFilterProp="label" />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('common:table.status')} valuePropName="checked" initialValue={true}>
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('rbac.roles.edit_title', { name: rbac.editingRole?.display_name || rbac.editingRole?.name || '' })}
                open={rbac.editRoleOpen}
                onOk={() => {
                    void rbac.submitEditRole();
                }}
                onCancel={rbac.closeEditRoleModal}
                confirmLoading={rbac.updateRolePending}
                destroyOnHidden={true}
            >
                <Form form={rbac.roleEditForm} layout="vertical" preserve={false}>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="description" label={t('common:table.description')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="permissions" label={t('rbac.roles.permissions')} rules={[{ required: true }]}> 
                        <Select mode="multiple" options={rbac.permissionOptions} optionFilterProp="label" />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('common:table.status')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('common:button.delete')}
                open={rbac.deleteRoleOpen}
                onOk={rbac.submitDeleteRole}
                onCancel={rbac.closeDeleteRoleModal}
                confirmLoading={rbac.deleteRolePending}
                okButtonProps={{ danger: true }}
            >
                <Text>{t('rbac.roles.delete_confirm', { name: rbac.deletingRole?.display_name || rbac.deletingRole?.name || '' })}</Text>
            </Modal>

            <Modal
                title={t('rbac.bindings.add_title')}
                open={rbac.addBindingOpen}
                onOk={() => {
                    void rbac.submitAddBinding();
                }}
                onCancel={rbac.closeAddBindingModal}
                confirmLoading={rbac.createBindingPending}
                destroyOnHidden={true}
            >
                <Form form={rbac.bindingForm} layout="vertical" preserve={false}>
                    <Form.Item label={t('rbac.bindings.select_user')}>
                        <Input value={rbac.selectedUser?.display_name || rbac.selectedUser?.username || ''} readOnly />
                    </Form.Item>
                    <Form.Item name="role_id" label={t('rbac.bindings.role')} rules={[{ required: true }]}> 
                        <Select options={roleOptions} optionFilterProp="label" showSearch />
                    </Form.Item>
                    <Form.Item name="scope_type" label={t('rbac.bindings.scope_type')} rules={[{ required: true }]} initialValue="global">
                        <Select options={scopeOptions} />
                    </Form.Item>
                    <Form.Item name="scope_id" label={t('rbac.bindings.scope_id')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="allowed_environments" label={t('rbac.bindings.allowed_envs')}>
                        <Select mode="multiple" options={environmentOptions} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
