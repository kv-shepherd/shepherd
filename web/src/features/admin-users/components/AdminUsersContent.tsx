'use client';

import {
    Button,
    Card,
    DatePicker,
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
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined, TeamOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useAdminUsersController } from '../hooks/useAdminUsersController';
import {
    MEMBER_ROLE_VALUES,
    type RateLimitUserStatus,
    type SystemMember,
    type SystemMemberRoleUpdateRequest,
    type User,
} from '../types';

const { Title, Text } = Typography;

export function AdminUsersContent() {
    const { t } = useTranslation(['admin', 'common']);
    const users = useAdminUsersController({ t });
    const [selectedRateLimitUserID, setSelectedRateLimitUserID] = useState<string>('');
    const [exemptionOpen, setExemptionOpen] = useState(false);
    const [overrideOpen, setOverrideOpen] = useState(false);
    const [exemptionForm] = Form.useForm<{
        reason?: string;
        expires_at?: dayjs.Dayjs | null;
    }>();
    const [overrideForm] = Form.useForm<{
        max_pending_parents?: number | null;
        max_pending_children?: number | null;
        cooldown_seconds?: number | null;
        reason?: string;
    }>();

    const usersColumns: ColumnsType<User> = [
        {
            title: t('users.table.username'),
            dataIndex: 'username',
            key: 'username',
            render: (username: string, record: User) => (
                <div>
                    <Text strong>{record.display_name || username}</Text>
                    <br />
                    <Text type="secondary" style={{ fontSize: 12 }}>{username}</Text>
                </div>
            ),
        },
        {
            title: t('users.table.email'),
            dataIndex: 'email',
            key: 'email',
            render: (email: string | undefined) => email || '—',
        },
        {
            title: t('users.table.roles'),
            dataIndex: 'roles',
            key: 'roles',
            render: (roles: string[] | undefined) => (
                <Space wrap>
                    {(roles ?? []).map((role) => (
                        <Tag key={role}>{role}</Tag>
                    ))}
                </Space>
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
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 170,
            render: (createdAt: string) => dayjs(createdAt).format('YYYY-MM-DD HH:mm'),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 180,
            render: (_, record: User) => (
                <Space>
                    <Button
                        size="small"
                        icon={<EditOutlined />}
                        onClick={() => users.openEditUserModal(record)}
                    >
                        {t('common:button.edit')}
                    </Button>
                    <Popconfirm
                        title={t('users.directory.delete_confirm', { username: record.username })}
                        onConfirm={() => users.deleteUser(record.id)}
                        okText={t('common:button.confirm')}
                        cancelText={t('common:button.cancel')}
                    >
                        <Button
                            size="small"
                            danger
                            icon={<DeleteOutlined />}
                            loading={users.deleteUserPending && users.deletingUserId === record.id}
                        >
                            {t('common:button.delete')}
                        </Button>
                    </Popconfirm>
                </Space>
            ),
        },
    ];

    const memberColumns: ColumnsType<SystemMember> = [
        {
            title: t('users.table.username'),
            dataIndex: 'username',
            key: 'username',
            render: (username: string, record: SystemMember) => (
                <div>
                    <Text strong>{record.display_name || username}</Text>
                    <br />
                    <Text type="secondary" style={{ fontSize: 12 }}>{username}</Text>
                </div>
            ),
        },
        {
            title: t('users.table.email'),
            dataIndex: 'email',
            key: 'email',
            render: (email: string | undefined) => email || '—',
        },
        {
            title: t('users.members.role'),
            dataIndex: 'role',
            key: 'role',
            width: 220,
            render: (role: SystemMember['role'], record: SystemMember) => (
                <Select
                    value={role}
                    options={memberRoleOptions}
                    style={{ width: 170 }}
                    onChange={(nextRole) => users.updateMemberRole(
                        record.user_id,
                        nextRole as NonNullable<SystemMemberRoleUpdateRequest['role']>
                    )}
                    loading={users.updatePending}
                />
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 120,
            render: (_, record: SystemMember) => (
                <Popconfirm
                    title={t('users.members.remove_confirm', { username: record.username })}
                    onConfirm={() => users.removeMember(record.user_id)}
                    okText={t('common:button.confirm')}
                    cancelText={t('common:button.cancel')}
                >
                    <Button danger size="small" loading={users.removePending}>
                        {t('common:button.delete')}
                    </Button>
                </Popconfirm>
            ),
        },
    ];

    const rateLimitColumns: ColumnsType<RateLimitUserStatus> = [
        {
            title: t('users.rate_limit.user_id'),
            dataIndex: 'user_id',
            key: 'user_id',
            render: (userID: string) => <Text code>{userID}</Text>,
        },
        {
            title: t('users.rate_limit.exempted'),
            dataIndex: 'exempted',
            key: 'exempted',
            width: 110,
            render: (exempted: boolean) => (
                <Tag color={exempted ? 'green' : 'default'}>
                    {exempted ? t('users.rate_limit.exempted_yes') : t('users.rate_limit.exempted_no')}
                </Tag>
            ),
        },
        {
            title: t('users.rate_limit.effective'),
            key: 'effective',
            render: (_, record) => (
                <Space direction="vertical" size={0}>
                    <Text type="secondary">{t('users.rate_limit.max_parents')}: {record.effective_max_pending_parents}</Text>
                    <Text type="secondary">{t('users.rate_limit.max_children')}: {record.effective_max_pending_children}</Text>
                    <Text type="secondary">{t('users.rate_limit.cooldown')}: {record.effective_cooldown_seconds}s</Text>
                </Space>
            ),
        },
        {
            title: t('users.rate_limit.current'),
            key: 'current',
            render: (_, record) => (
                <Space direction="vertical" size={0}>
                    <Text type="secondary">{t('users.rate_limit.pending_parents')}: {record.current_pending_parents}</Text>
                    <Text type="secondary">{t('users.rate_limit.pending_children')}: {record.current_pending_children}</Text>
                    <Text type="secondary">{t('users.rate_limit.remaining')}: {record.cooldown_remaining_seconds}s</Text>
                </Space>
            ),
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 320,
            render: (_, record) => (
                <Space wrap>
                    <Button
                        size="small"
                        onClick={() => {
                            setSelectedRateLimitUserID(record.user_id);
                            exemptionForm.resetFields();
                            setExemptionOpen(true);
                        }}
                    >
                        {t('users.rate_limit.add_exemption')}
                    </Button>
                    <Button
                        size="small"
                        onClick={() => {
                            setSelectedRateLimitUserID(record.user_id);
                            overrideForm.setFieldsValue({
                                max_pending_parents: record.effective_max_pending_parents,
                                max_pending_children: record.effective_max_pending_children,
                                cooldown_seconds: record.effective_cooldown_seconds,
                            });
                            setOverrideOpen(true);
                        }}
                    >
                        {t('users.rate_limit.override')}
                    </Button>
                    <Popconfirm
                        title={t('users.rate_limit.remove_exemption_confirm')}
                        onConfirm={() => users.removeRateLimitExemption(record.user_id)}
                        okText={t('common:button.confirm')}
                        cancelText={t('common:button.cancel')}
                    >
                        <Button size="small" danger disabled={!record.exempted} loading={users.rateLimitMutationPending}>
                            {t('users.rate_limit.remove_exemption')}
                        </Button>
                    </Popconfirm>
                </Space>
            ),
        },
    ];

    const existingMemberUserIDs = useMemo(
        () => new Set((users.members?.items ?? []).map((member) => member.user_id)),
        [users.members?.items]
    );

    const addableUsers = useMemo(
        () => (users.users?.items ?? []).filter((u) => !existingMemberUserIDs.has(u.id)),
        [existingMemberUserIDs, users.users?.items]
    );
    const editingUser = useMemo(
        () => (users.users?.items ?? []).find((u) => u.id === users.editingUserId),
        [users.editingUserId, users.users?.items]
    );
    const memberRoleOptions = MEMBER_ROLE_VALUES.map((role) => ({
        value: role,
        label: t(`users.members.role_option.${role}`, { defaultValue: role }),
    }));

    return (
        <div>
            {users.messageContextHolder}
            <div style={{ marginBottom: 24 }}>
                <Title level={4} style={{ margin: 0 }}>{t('users.title')}</Title>
                <Text type="secondary">{t('users.subtitle')}</Text>
            </div>

            <Card style={{ borderRadius: 12, marginBottom: 16 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0}>
                        <Text strong>{t('users.directory.title')}</Text>
                        <Text type="secondary">{t('users.directory.subtitle')}</Text>
                    </Space>
                    <Space>
                        <Button icon={<ReloadOutlined />} onClick={() => users.refetchUsers()}>
                            {t('common:button.refresh')}
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} onClick={users.openCreateUserModal}>
                            {t('users.directory.add')}
                        </Button>
                    </Space>
                </Space>
                <Table<User>
                    style={{ marginTop: 16 }}
                    rowKey="id"
                    columns={usersColumns}
                    dataSource={users.users?.items ?? []}
                    loading={users.usersLoading}
                    pagination={{
                        current: users.page,
                        pageSize: users.perPage,
                        total: users.users?.pagination?.total ?? 0,
                        showSizeChanger: true,
                        showTotal: (total) => t('common:table.total', { total }),
                        onChange: (nextPage, nextPageSize) => {
                            users.setPage(nextPage);
                            users.setPerPage(nextPageSize);
                        },
                    }}
                />
            </Card>

            <Modal
                title={t('users.directory.add_title')}
                open={users.createUserOpen}
                onOk={() => {
                    void users.submitCreateUser();
                }}
                onCancel={users.closeCreateUserModal}
                confirmLoading={users.createUserPending}
                destroyOnHidden={true}
            >
                <Form form={users.createUserForm} layout="vertical" preserve={false}>
                    <Form.Item
                        name="username"
                        label={t('common:auth.username')}
                        rules={[
                            { required: true, message: t('common:validation.username_required') },
                            { min: 2, message: t('common:validation.username_min') },
                        ]}
                    >
                        <Input autoComplete="off" />
                    </Form.Item>
                    <Form.Item
                        name="password"
                        label={t('common:auth.password')}
                        rules={[
                            { required: true, message: t('common:validation.password_required') },
                            { min: 8, message: t('common:validation.password_min') },
                        ]}
                    >
                        <Input.Password autoComplete="new-password" />
                    </Form.Item>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="email" label={t('users.table.email')}>
                        <Input type="email" />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('common:table.status')} valuePropName="checked" initialValue={true}>
                        <Switch />
                    </Form.Item>
                    <Form.Item
                        name="force_password_change"
                        label={t('users.directory.force_password_change')}
                        valuePropName="checked"
                        initialValue={true}
                    >
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('users.directory.edit_title', {
                    username: editingUser?.display_name || editingUser?.username || '',
                })}
                open={users.editUserOpen}
                onOk={() => {
                    void users.submitEditUser();
                }}
                onCancel={users.closeEditUserModal}
                confirmLoading={users.updateUserPending}
                destroyOnHidden={true}
            >
                <Form form={users.editUserForm} layout="vertical" preserve={false}>
                    <Form.Item name="display_name" label={t('common:table.display_name')}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="email" label={t('users.table.email')}>
                        <Input type="email" />
                    </Form.Item>
                    <Form.Item
                        name="password"
                        label={t('users.directory.password')}
                        rules={[
                            { min: 8, message: t('common:validation.password_min') },
                        ]}
                    >
                        <Input.Password autoComplete="new-password" allowClear={true} />
                    </Form.Item>
                    <Form.Item name="enabled" label={t('common:table.status')} valuePropName="checked">
                        <Switch />
                    </Form.Item>
                    <Form.Item
                        name="force_password_change"
                        label={t('users.directory.force_password_change')}
                        valuePropName="checked"
                    >
                        <Switch />
                    </Form.Item>
                </Form>
            </Modal>

            <Card style={{ borderRadius: 12 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0}>
                        <Text strong>{t('users.members.title')}</Text>
                        <Text type="secondary">{t('users.members.subtitle')}</Text>
                    </Space>
                    <Space>
                        <Button icon={<ReloadOutlined />} onClick={() => users.refetchMembers()} disabled={!users.selectedSystemId}>
                            {t('common:button.refresh')}
                        </Button>
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            onClick={users.openAddModal}
                            disabled={!users.selectedSystemId}
                        >
                            {t('users.members.add')}
                        </Button>
                    </Space>
                </Space>

                <Space align="center" style={{ marginTop: 16, marginBottom: 16 }}>
                    <TeamOutlined />
                    <Text>{t('users.members.select_system')}</Text>
                    <Select
                        style={{ minWidth: 280 }}
                        loading={users.systemsLoading}
                        value={users.selectedSystemId}
                        placeholder={t('users.members.select_system_placeholder')}
                        onChange={(value) => users.setSelectedSystemId(value)}
                        options={users.systems.map((system) => ({ value: system.id, label: system.name }))}
                        showSearch
                        optionFilterProp="label"
                    />
                </Space>

                <Table<SystemMember>
                    rowKey="user_id"
                    columns={memberColumns}
                    dataSource={users.members?.items ?? []}
                    loading={users.membersLoading}
                    locale={{
                        emptyText: users.selectedSystemId ? t('common:message.no_data') : t('users.members.select_system_first'),
                    }}
                    pagination={false}
                />
            </Card>

            <Card style={{ borderRadius: 12, marginTop: 16 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0}>
                        <Text strong>{t('users.rate_limit.title')}</Text>
                        <Text type="secondary">{t('users.rate_limit.subtitle')}</Text>
                    </Space>
                    <Button icon={<ReloadOutlined />} onClick={() => users.refetchRateLimitStatus()}>
                        {t('common:button.refresh')}
                    </Button>
                </Space>
                <Table<RateLimitUserStatus>
                    style={{ marginTop: 16 }}
                    rowKey="user_id"
                    columns={rateLimitColumns}
                    dataSource={users.rateLimitStatus?.items ?? []}
                    loading={users.rateLimitLoading}
                    pagination={false}
                />
            </Card>

            <Modal
                title={t('users.members.add_title')}
                open={users.addOpen}
                onOk={() => {
                    void users.submitAddMember();
                }}
                onCancel={users.closeAddModal}
                confirmLoading={users.addPending}
                destroyOnHidden={true}
            >
                <Form form={users.addForm} layout="vertical" preserve={false}>
                    <Form.Item
                        name="user_id"
                        label={t('users.members.select_user')}
                        rules={[{ required: true, message: t('users.members.validation.user_required') }]}
                    >
                        <Select
                            showSearch
                            optionFilterProp="label"
                            placeholder={t('users.members.select_user_placeholder')}
                            options={addableUsers.map((u) => ({
                                value: u.id,
                                label: `${u.username}${u.display_name ? ` (${u.display_name})` : ''}`,
                            }))}
                            notFoundContent={t('users.members.no_addable_users')}
                        />
                    </Form.Item>
                    <Form.Item
                        name="role"
                        label={t('users.members.select_role')}
                        rules={[{ required: true, message: t('users.members.validation.role_required') }]}
                        initialValue="viewer"
                    >
                        <Select options={memberRoleOptions} />
                    </Form.Item>
                    <Form.Item label={t('users.members.note_label')}>
                        <Input.TextArea rows={3} value={t('users.members.note')} readOnly />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('users.rate_limit.add_exemption')}
                open={exemptionOpen}
                onCancel={() => {
                    setExemptionOpen(false);
                    setSelectedRateLimitUserID('');
                    exemptionForm.resetFields();
                }}
                onOk={() => {
                    void exemptionForm.validateFields().then((values) => {
                        users.applyRateLimitExemption({
                            user_id: selectedRateLimitUserID,
                            reason: values.reason || '',
                            expires_at: values.expires_at ? values.expires_at.toISOString() : null,
                        });
                        setExemptionOpen(false);
                        setSelectedRateLimitUserID('');
                        exemptionForm.resetFields();
                    });
                }}
                confirmLoading={users.rateLimitMutationPending}
                destroyOnHidden={true}
            >
                <Form form={exemptionForm} layout="vertical" preserve={false}>
                    <Form.Item label={t('users.rate_limit.user_id')}>
                        <Input value={selectedRateLimitUserID} readOnly />
                    </Form.Item>
                    <Form.Item name="reason" label={t('users.rate_limit.reason')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                    <Form.Item name="expires_at" label={t('users.rate_limit.expires_at')}>
                        <DatePicker showTime style={{ width: '100%' }} />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('users.rate_limit.override')}
                open={overrideOpen}
                onCancel={() => {
                    setOverrideOpen(false);
                    setSelectedRateLimitUserID('');
                    overrideForm.resetFields();
                }}
                onOk={() => {
                    void overrideForm.validateFields().then((values) => {
                        users.updateRateLimitOverride(selectedRateLimitUserID, {
                            max_pending_parents: values.max_pending_parents ?? null,
                            max_pending_children: values.max_pending_children ?? null,
                            cooldown_seconds: values.cooldown_seconds ?? null,
                            reason: values.reason || '',
                        });
                        setOverrideOpen(false);
                        setSelectedRateLimitUserID('');
                        overrideForm.resetFields();
                    });
                }}
                confirmLoading={users.rateLimitMutationPending}
                destroyOnHidden={true}
            >
                <Form form={overrideForm} layout="vertical" preserve={false}>
                    <Form.Item label={t('users.rate_limit.user_id')}>
                        <Input value={selectedRateLimitUserID} readOnly />
                    </Form.Item>
                    <Form.Item name="max_pending_parents" label={t('users.rate_limit.max_parents')}>
                        <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="max_pending_children" label={t('users.rate_limit.max_children')}>
                        <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="cooldown_seconds" label={t('users.rate_limit.cooldown')}>
                        <InputNumber min={0} style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="reason" label={t('users.rate_limit.reason')}>
                        <Input.TextArea rows={3} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
