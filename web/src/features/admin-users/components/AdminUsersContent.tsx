'use client';

import {
    Alert,
    AutoComplete,
    Button,
    DatePicker,
    Drawer,
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
import type { Dayjs } from 'dayjs';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import { localizeRoleLabel } from '@/features/rbac-shared/roleCatalogI18n';
import {
    AccessControlGlyph,
    QueueReviewGlyph,
    RateLimitGaugeGlyph,
    UserDirectoryGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { useAdminUsersController } from '../hooks/useAdminUsersController';
import {
    MEMBER_ROLE_VALUES,
    type GlobalRoleBinding,
    type RateLimitUserStatus,
    type SystemMember,
    type SystemMemberRoleUpdateRequest,
    type User,
} from '../types';

const { Text } = Typography;
const EMPTY_VALUE = '—';
const USER_ROLE_BINDING_SCOPE_VALUES = ['global', 'system', 'service', 'vm'] as const;
const USER_ROLE_BINDING_ENV_VALUES = ['test', 'prod'] as const;

export function AdminUsersContent() {
    const { t } = useTranslation(['admin', 'common']);
    const users = useAdminUsersController({ t });
    const [selectedRateLimitUserID, setSelectedRateLimitUserID] = useState<string>('');
    const [exemptionOpen, setExemptionOpen] = useState(false);
    const [overrideOpen, setOverrideOpen] = useState(false);
    const [exemptionForm] = Form.useForm<{
        reason?: string;
        expires_at?: Dayjs | null;
    }>();
    const [overrideForm] = Form.useForm<{
        max_pending_parents?: number | null;
        max_pending_children?: number | null;
        cooldown_seconds?: number | null;
        reason?: string;
    }>();
    const userItems = users.users?.items ?? [];
    const memberItems = users.members?.items ?? [];
    const rateLimitItems = users.rateLimitStatus?.items ?? [];
    const roleCatalog = useMemo(
        () => users.roles?.items ?? [],
        [users.roles?.items]
    );
    const selectedSystem = users.systems.find((system) => system.id === users.selectedSystemId);
    const selectedRateLimitRecord = rateLimitItems.find((item) => item.user_id === selectedRateLimitUserID);
    const roleDisplayByName = useMemo(
        () => new Map(roleCatalog.map((role) => [role.name, localizeRoleLabel(t, role)])),
        [roleCatalog, t]
    );
    const roleDisplayById = useMemo(
        () => new Map(roleCatalog.map((role) => [role.id, localizeRoleLabel(t, role)])),
        [roleCatalog, t]
    );
    const roleBindingScopeType = Form.useWatch('scope_type', users.roleBindingCreateForm);
    const roleBindingScopeOptions = useMemo(
        () => USER_ROLE_BINDING_SCOPE_VALUES.map((scope) => ({
            value: scope,
            label: t(`rbac.scope.${scope}`, { defaultValue: scope }),
        })),
        [t]
    );
    const roleBindingEnvironmentOptions = useMemo(
        () => USER_ROLE_BINDING_ENV_VALUES.map((env) => ({
            value: env,
            label: t(`rbac.env.${env}`, { defaultValue: env }),
        })),
        [t]
    );
    const roleBindingScopeTargetOptions = users.scopeTargetOptionsByType?.[roleBindingScopeType || 'global'] ?? [];
    const roleBindingScopeTargetLoading = Boolean(users.scopeTargetLoadingByType?.[roleBindingScopeType || 'global']);
    const totalUsers = users.users?.pagination?.total ?? userItems.length;
    const enabledUsers = userItems.filter((user) => user.enabled).length;
    const trackedRateLimitUsers = rateLimitItems.length;
    const exemptedUsers = rateLimitItems.filter((item) => item.exempted).length;

    const renderUserIdentity = (
        record: Pick<User, 'username' | 'display_name' | 'email' | 'id'>
        | Pick<SystemMember, 'username' | 'display_name' | 'email' | 'user_id'>
        | Pick<RateLimitUserStatus, 'username' | 'display_name' | 'email' | 'user_id'>
    ) => {
        const username = 'username' in record ? record.username : undefined;
        const displayName = 'display_name' in record ? record.display_name : undefined;
        const email = 'email' in record ? record.email : undefined;
        const identityId = 'id' in record ? record.id : record.user_id;
        const primary = displayName?.trim() || username || identityId;
        const secondary = username && username !== primary ? username : identityId;

        return (
            <Space direction="vertical" size={0}>
                <Text strong>{primary}</Text>
                {secondary ? <Text type="secondary" style={{ fontSize: 12 }}>{secondary}</Text> : null}
                <Text type="secondary" style={{ fontSize: 12 }}>{email || t('users.common.no_email')}</Text>
            </Space>
        );
    };

    const renderRoleTags = (roles: string[] | undefined) => {
        if (!roles || roles.length === 0) {
            return <Text type="secondary">{t('users.directory.no_roles')}</Text>;
        }
        return (
            <Space wrap>
                {roles.map((roleName) => {
                    const label = roleDisplayByName.get(roleName) || roleName;
                    return (
                        <Tag key={roleName} title={roleName} color="processing">
                            {label}
                        </Tag>
                    );
                })}
            </Space>
        );
    };

    const usersColumns: ColumnsType<User> = [
        {
            title: t('users.table.username'),
            dataIndex: 'username',
            key: 'username',
            render: (_, record: User) => renderUserIdentity(record),
        },
        {
            title: t('users.table.email'),
            dataIndex: 'email',
            key: 'email',
            render: (email: string | undefined) => email || EMPTY_VALUE,
        },
        {
            title: t('users.table.roles'),
            dataIndex: 'roles',
            key: 'roles',
            render: (roles: string[] | undefined) => renderRoleTags(roles),
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
            render: (createdAt: string) => <LocalDateTimeText value={createdAt} />,
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 220,
            render: (_, record: User) => (
                <Space size={4} wrap>
                    <Button
                        type="link"
                        size="small"
                        icon={<EditOutlined />}
                        data-testid={`user-action-edit-${record.id}`}
                        onClick={() => users.openEditUserModal(record)}
                    >
                        {t('common:button.edit')}
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        icon={<TeamOutlined />}
                        data-testid={`user-action-role-bindings-${record.id}`}
                        onClick={() => users.openRoleBindingsModal(record)}
                    >
                        {t('users.directory.manage_access')}
                    </Button>
                    <Popconfirm
                        title={t('users.directory.delete_confirm', { username: record.username })}
                        onConfirm={() => users.deleteUser(record.id)}
                        okText={t('common:button.confirm')}
                        cancelText={t('common:button.cancel')}
                    >
                        <Button
                            type="link"
                            size="small"
                            danger
                            icon={<DeleteOutlined />}
                            data-testid={`user-action-delete-${record.id}`}
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
            render: (_, record: SystemMember) => renderUserIdentity(record),
        },
        {
            title: t('users.table.email'),
            dataIndex: 'email',
            key: 'email',
            render: (email: string | undefined) => email || EMPTY_VALUE,
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
                    data-testid={`member-role-select-${record.user_id}`}
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
                    <Button type="link" danger size="small" data-testid={`member-action-remove-${record.user_id}`} loading={users.removePending}>
                        {t('common:button.delete')}
                    </Button>
                </Popconfirm>
            ),
        },
    ];

    const rateLimitColumns: ColumnsType<RateLimitUserStatus> = [
        {
            title: t('users.rate_limit.user'),
            dataIndex: 'user_id',
            key: 'user_id',
            render: (_, record: RateLimitUserStatus) => renderUserIdentity(record),
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
                        type="link"
                        size="small"
                        data-testid={`ratelimit-action-exempt-${record.user_id}`}
                        onClick={() => {
                            setSelectedRateLimitUserID(record.user_id);
                            exemptionForm.resetFields();
                            setExemptionOpen(true);
                        }}
                    >
                        {t('users.rate_limit.add_exemption')}
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        data-testid={`rate-limit-user-action-edit-${record.user_id}`}
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
                        {t('users.rate_limit.edit_limits')}
                    </Button>
                    <Popconfirm
                        title={t('users.rate_limit.remove_exemption_confirm')}
                        onConfirm={() => users.removeRateLimitExemption(record.user_id)}
                        okText={t('common:button.confirm')}
                        cancelText={t('common:button.cancel')}
                    >
                        <Button type="link" size="small" danger data-testid={`rate-limit-exemption-action-delete-${record.user_id}`} disabled={!record.exempted} loading={users.rateLimitMutationPending}>
                            {t('users.rate_limit.remove_exemption')}
                        </Button>
                    </Popconfirm>
                </Space>
            ),
        },
    ];

    const editingUser = useMemo(
        () => (users.users?.items ?? []).find((u) => u.id === users.editingUserId),
        [users.editingUserId, users.users?.items]
    );
    const memberCandidateOptions = useMemo(
        () => (users.memberCandidates?.items ?? []).map((user) => ({
            value: user.id,
            label: user.display_name
                ? `${user.display_name} (${user.username})`
                : user.username,
        })),
        [users.memberCandidates?.items]
    );
    const memberRoleOptions = MEMBER_ROLE_VALUES.map((role) => ({
        value: role,
        label: t(`users.members.role_option.${role}`, { defaultValue: role }),
    }));

    return (
        <div data-testid="admin-users-page">
            {users.messageContextHolder}
            <PageHeader
                title={t('users.title')}
                subtitle={t('users.subtitle')}
            />

            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('users.summary.directory_title')}
                    value={totalUsers}
                    description={t('users.summary.directory_description')}
                    visual={<UserDirectoryGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('users.summary.enabled_title')}
                    value={enabledUsers}
                    description={t('users.summary.enabled_description')}
                    visual={<AccessControlGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('users.summary.members_title', { system: selectedSystem?.name || t('users.members.select_system_placeholder') })}
                    value={users.selectedSystemId ? memberItems.length : users.systems.length}
                    description={users.selectedSystemId
                        ? t('users.summary.members_description_selected', { system: selectedSystem?.name || EMPTY_VALUE })
                        : t('users.summary.members_description')}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#D97706"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('users.summary.rate_limit_title')}
                    value={trackedRateLimitUsers}
                    description={t('users.summary.rate_limit_description', { exempted: exemptedUsers })}
                    visual={<RateLimitGaugeGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface style={{ marginBottom: 16 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0}>
                        <Text strong>{t('users.directory.title')}</Text>
                        <Text type="secondary">{t('users.directory.subtitle')}</Text>
                    </Space>
                    <Space>
                        <Button icon={<ReloadOutlined />} onClick={() => users.refetchUsers()}>
                            {t('common:button.refresh')}
                        </Button>
                        <Button type="primary" icon={<PlusOutlined />} data-testid="user-create-button" onClick={users.openCreateUserModal}>
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
                    locale={{
                        emptyText: (
                            <div style={{ padding: 40 }}>
                                <ActionEmptyState
                                    title={t('users.directory.empty')}
                                    description={t('users.directory.empty_description')}
                                    visual={<UserDirectoryGlyph className="action-empty-state__art" />}
                                    actions={(
                                        <Button type="primary" icon={<PlusOutlined />} onClick={users.openCreateUserModal}>
                                            {t('users.directory.add')}
                                        </Button>
                                    )}
                                />
                            </div>
                        ),
                    }}
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
            </PageSurface>

            <Modal
                title={t('users.directory.add_title')}
                open={users.createUserOpen}
                onOk={() => {
                    void users.submitCreateUser();
                }}
                onCancel={users.closeCreateUserModal}
                confirmLoading={users.createUserPending}
                forceRender={true}
                maskClosable={false}
                keyboard={false}
                data-testid="user-create-modal"
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
                forceRender={true}
                maskClosable={false}
                keyboard={false}
                data-testid="user-edit-modal"
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

            <PageSurface>
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
                            data-testid="member-add-button"
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
                        data-testid="users-system-selector"
                        onChange={(value) => users.setSelectedSystemId(value)}
                        options={users.systems.map((system) => ({ value: system.id, label: system.name }))}
                        showSearch
                        optionFilterProp="label"
                    />
                </Space>

                <Table<SystemMember>
                    rowKey="user_id"
                    columns={memberColumns}
                    dataSource={memberItems}
                    loading={users.membersLoading}
                    locale={{
                        emptyText: users.selectedSystemId ? (
                            <div style={{ padding: 40 }}>
                                <ActionEmptyState
                                    compact={true}
                                    title={t('users.members.empty')}
                                    description={t('users.members.empty_description', { system: selectedSystem?.name || EMPTY_VALUE })}
                                    visual={<AccessControlGlyph className="action-empty-state__art" />}
                                    actions={(
                                        <Button type="primary" size="small" icon={<PlusOutlined />} onClick={users.openAddModal}>
                                            {t('users.members.add')}
                                        </Button>
                                    )}
                                />
                            </div>
                        ) : (
                            <div style={{ padding: 40 }}>
                                <ActionEmptyState
                                    compact={true}
                                    title={t('users.members.select_system_first')}
                                    description={t('users.members.select_system_help')}
                                    visual={<QueueReviewGlyph className="action-empty-state__art" />}
                                />
                            </div>
                        ),
                    }}
                    pagination={false}
                />
            </PageSurface>

            <PageSurface style={{ marginTop: 16 }}>
                <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0}>
                        <Text strong>{t('users.rate_limit.title')}</Text>
                        <Text type="secondary">{t('users.rate_limit.subtitle')}</Text>
                    </Space>
                    <Space>
                        <Button icon={<ReloadOutlined />} onClick={() => users.refetchRateLimitStatus()}>
                            {t('common:button.refresh')}
                        </Button>
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            data-testid="rate-limit-exemption-create-button"
                            onClick={() => {
                                exemptionForm.resetFields();
                                setExemptionOpen(true);
                            }}
                        >
                            {t('users.rate_limit.add_exemption')}
                        </Button>
                    </Space>
                </Space>
                <Table<RateLimitUserStatus>
                    style={{ marginTop: 16 }}
                    rowKey="user_id"
                    columns={rateLimitColumns}
                    dataSource={rateLimitItems}
                    loading={users.rateLimitLoading}
                    locale={{
                        emptyText: (
                            <div style={{ padding: 40 }}>
                                <ActionEmptyState
                                    compact={true}
                                    title={t('users.rate_limit.empty')}
                                    description={t('users.rate_limit.empty_description')}
                                    visual={<RateLimitGaugeGlyph className="action-empty-state__art" />}
                                />
                            </div>
                        ),
                    }}
                    pagination={false}
                />
            </PageSurface>

            <Modal
                title={t('users.members.add_title')}
                open={users.addOpen}
                onOk={() => {
                    void users.submitAddMember();
                }}
                onCancel={users.closeAddModal}
                confirmLoading={users.addPending}
                forceRender={true}
                maskClosable={false}
                keyboard={false}
                data-testid="member-add-modal"
            >
                <Form form={users.addForm} layout="vertical" preserve={false}>
                    <Form.Item
                        name="user_id"
                        label={t('users.members.select_user')}
                        rules={[{ required: true, message: t('users.members.validation.user_required') }]}
                    >
                        <Select
                            showSearch
                            placeholder={t('users.members.select_user_placeholder')}
                            data-testid="member-candidate-user-select"
                            filterOption={false}
                            loading={users.memberCandidatesLoading}
                            searchValue={users.memberCandidateSearch}
                            onSearch={users.setMemberCandidateSearch}
                            options={memberCandidateOptions}
                            notFoundContent={
                                users.memberCandidatesLoading
                                    ? t('common:message.loading')
                                    : users.memberCandidateSearch.trim()
                                        ? t('users.members.no_search_results')
                                        : t('users.members.no_addable_users')
                            }
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
                forceRender={true}
                maskClosable={false}
                keyboard={false}
                data-testid="rate-limit-exemption-create-modal"
            >
                <Form form={exemptionForm} layout="vertical" preserve={false}>
                    <Form.Item label={t('users.rate_limit.user')}>
                        <Input
                            value={
                                selectedRateLimitRecord?.display_name?.trim()
                                || selectedRateLimitRecord?.username
                                || selectedRateLimitUserID
                            }
                            readOnly
                        />
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
                forceRender={true}
                maskClosable={false}
                keyboard={false}
                data-testid="rate-limit-user-edit-modal"
            >
                <Form form={overrideForm} layout="vertical" preserve={false}>
                    <Form.Item label={t('users.rate_limit.user')}>
                        <Input
                            value={
                                selectedRateLimitRecord?.display_name?.trim()
                                || selectedRateLimitRecord?.username
                                || selectedRateLimitUserID
                            }
                            readOnly
                        />
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
            {/* ── User Role Bindings Drawer ─────────────────────────────── */}
            <Drawer
                title={t('users.role_bindings.drawer_title')}
                open={Boolean(users.roleBindingsUserId)}
                onClose={users.closeRoleBindingsModal}
                width={700}
                zIndex={1000}
                destroyOnClose
                maskClosable={false}
                data-testid="user-role-bindings-page"
            >
                <Space direction="vertical" size={4} style={{ marginBottom: 16 }}>
                    <Text strong>{users.roleBindingsUserLabel || users.roleBindingsUserId}</Text>
                    <Text type="secondary">
                        {t('users.role_bindings.drawer_subtitle')}
                    </Text>
                </Space>
                <Alert
                    showIcon
                    type="info"
                    style={{ marginBottom: 16 }}
                    message={t('users.role_bindings.drawer_help_title')}
                    description={t('users.role_bindings.drawer_help_description')}
                />
                <Space style={{ marginBottom: 16, width: '100%', justifyContent: 'flex-end' }}>
                    <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        data-testid="role-binding-create-button"
                        onClick={users.openRoleBindingCreateModal}
                    >
                        {t('users.role_bindings.create')}
                    </Button>
                </Space>
                <Table<GlobalRoleBinding>
                    rowKey="id"
                    loading={users.roleBindingsLoading}
                    dataSource={users.roleBindings?.items ?? []}
                    pagination={false}
                    locale={{
                        emptyText: (
                            <div style={{ padding: 32 }}>
                                <ActionEmptyState
                                    compact={true}
                                    title={t('users.role_bindings.empty')}
                                    description={t('users.role_bindings.empty_description')}
                                    visual={<AccessControlGlyph className="action-empty-state__art" />}
                                    actions={(
                                        <Button type="primary" size="small" icon={<PlusOutlined />} onClick={users.openRoleBindingCreateModal}>
                                            {t('users.role_bindings.create')}
                                        </Button>
                                    )}
                                />
                            </div>
                        ),
                    }}
                    columns={[
                        {
                            title: t('users.role_bindings.role_name'),
                            dataIndex: 'role_name',
                            key: 'role_name',
                            render: (roleName: string, record: GlobalRoleBinding) => (
                                <Space direction="vertical" size={0}>
                                    <Text strong>{record.role_display_name || roleDisplayById.get(record.role_id) || roleName}</Text>
                                    {(record.role_display_name || roleDisplayById.get(record.role_id))
                                        && (record.role_display_name || roleDisplayById.get(record.role_id)) !== roleName ? (
                                            <Text type="secondary" style={{ fontSize: 12 }}>{roleName}</Text>
                                        ) : null}
                                </Space>
                            ),
                        },
                        {
                            title: t('users.role_bindings.scope'),
                            key: 'scope',
                            render: (_: unknown, record: GlobalRoleBinding) => (
                                <Space direction="vertical" size={0}>
                                    <Space size={8} wrap>
                                        <Tag>{t(`rbac.scope.${record.scope_type}`, { defaultValue: record.scope_type })}</Tag>
                                        {record.allowed_environments?.length ? record.allowed_environments.map((env) => (
                                            <Tag key={`${record.id}-${env}`} color="processing">
                                                {t(`rbac.env.${env}`, { defaultValue: env })}
                                            </Tag>
                                        )) : <Tag>{t('users.role_bindings.all_environments')}</Tag>}
                                    </Space>
                                    <Text type="secondary" style={{ fontSize: 12 }}>
                                        {record.scope_display_name || record.scope_id || t('users.role_bindings.global_scope')}
                                    </Text>
                                </Space>
                            ),
                        },
                        {
                            title: t('users.role_bindings.created_at'),
                            dataIndex: 'created_at',
                            key: 'created_at',
                            render: (val: string) => <LocalDateTimeText value={val} />,
                        },
                        {
                            title: t('common:table.actions'),
                            key: 'actions',
                            render: (_: unknown, record: GlobalRoleBinding) => (
                                <Popconfirm
                                    title={t('users.role_bindings.delete_confirm')}
                                    onConfirm={() => users.deleteRoleBinding(record.id)}
                                    okText={t('common:button.confirm')}
                                    cancelText={t('common:button.cancel')}
                                >
                                    <Button
                                        type="link"
                                        size="small"
                                        danger
                                        icon={<DeleteOutlined />}
                                        loading={users.deletingBindingId === record.id && users.deleteRoleBindingPending}
                                        data-testid={`role-binding-action-delete-${record.id}`}
                                    />
                                </Popconfirm>
                            ),
                        },
                    ]}
                />
            </Drawer>

            {/* ── Create Role Binding Modal ────────────────────────────── */}
            <Modal
                title={t('users.role_bindings.create_modal_title')}
                open={users.roleBindingCreateOpen}
                onCancel={users.closeRoleBindingCreateModal}
                onOk={users.submitCreateRoleBinding}
                confirmLoading={users.createRoleBindingPending}
                width={560}
                zIndex={1100}
                maskClosable={false}
                keyboard={false}
                destroyOnHidden={true}
                forceRender={true}
                data-testid="role-binding-create-modal"
            >
                <Alert
                    showIcon
                    type="info"
                    style={{ marginBottom: 16 }}
                    message={t('users.role_bindings.create_help_title')}
                    description={t('users.role_bindings.create_help_description')}
                />
                <Form form={users.roleBindingCreateForm} layout="vertical" preserve={false}>
                    <Form.Item label={t('users.role_bindings.target_user')}>
                        <Input value={users.roleBindingsUserLabel || users.roleBindingsUserId} readOnly />
                    </Form.Item>
                    <Form.Item
                        name="role_id"
                        label={t('users.role_bindings.role')}
                        rules={[{ required: true }]}
                    >
                        <Select
                            placeholder={t('users.role_bindings.select_role')}
                            loading={users.rolesLoading}
                            options={(users.roles?.items ?? []).map((r) => ({
                                value: r.id,
                                label: localizeRoleLabel(t, r),
                            }))}
                        />
                    </Form.Item>
                    <Form.Item
                        name="scope_type"
                        label={t('users.role_bindings.scope_type')}
                        initialValue="global"
                        rules={[{ required: true }]}
                    >
                        <Select
                            options={roleBindingScopeOptions}
                            onChange={(value) => {
                                users.roleBindingCreateForm.setFieldsValue({ scope_type: value, scope_id: undefined });
                            }}
                        />
                    </Form.Item>
                    {roleBindingScopeType && roleBindingScopeType !== 'global' ? (
                        <Form.Item
                            name="scope_id"
                            label={t('users.role_bindings.scope_id')}
                            extra={t('users.role_bindings.scope_id_help', {
                                scope: t(`rbac.scope.${roleBindingScopeType}`, { defaultValue: roleBindingScopeType }),
                            })}
                        >
                            <AutoComplete
                                options={roleBindingScopeTargetOptions}
                                allowClear
                                placeholder={t('users.role_bindings.scope_id_placeholder')}
                                filterOption={(inputValue, option) => {
                                    const label = String(option?.label ?? '').toLowerCase();
                                    const value = String(option?.value ?? '').toLowerCase();
                                    const search = inputValue.trim().toLowerCase();
                                    return label.includes(search) || value.includes(search);
                                }}
                                notFoundContent={roleBindingScopeTargetLoading
                                    ? t('common:status.loading', { defaultValue: 'Loading…' })
                                    : t('users.role_bindings.scope_target_empty')}
                            />
                        </Form.Item>
                    ) : null}
                    <Form.Item name="allowed_environments" label={t('users.role_bindings.allowed_envs')}>
                        <Select mode="multiple" options={roleBindingEnvironmentOptions} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
