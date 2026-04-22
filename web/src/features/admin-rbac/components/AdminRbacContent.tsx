'use client';

import {
    Alert,
    AutoComplete,
    Button,
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
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    localizeRoleAssignmentPolicy,
    localizeRoleDescription,
    localizeRoleLabel,
} from '@/features/rbac-shared/roleCatalogI18n';
import {
    getRoleAccessTagColor,
    isPrivilegedRole,
    isPrivilegedRoleBinding,
} from '@/features/rbac-shared/privilegedAccess';
import type { ScopeTargetOption } from '@/features/rbac-shared/useScopeTargetCatalog';
import {
    AccessControlGlyph,
    RoleCatalogGlyph,
    UserDirectoryGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import { hasPermission } from '@/lib/auth/permissions';
import { useAuthStore } from '@/stores/auth';
import { useAdminRbacController } from '../hooks/useAdminRbacController';
import {
    ENVIRONMENT_VALUES,
    RBAC_SCOPE_VALUES,
    type GlobalRoleBinding,
    type Permission,
    type Role,
} from '../types';

const { Text } = Typography;
const EMPTY_VALUE = '—';
function permissionCatalogTranslationKey(permissionKey: string) {
    return permissionKey.replace(/[^a-zA-Z0-9]+/g, '_').replace(/^_+|_+$/g, '').toLowerCase();
}

export function AdminRbacContent() {
    const { t } = useTranslation(['admin', 'common']);
    const searchParams = useSearchParams();
    const currentUser = useAuthStore((state) => state.user);
    const canManageRbac = hasPermission(currentUser, 'rbac:manage');
    const rbac = useAdminRbacController({ t });
    const { selectedUserId, selectUser } = rbac;
    const [quickSearch, setQuickSearch] = useState('');
    const [quickSearchDraft, setQuickSearchDraft] = useState('');
    const [filtersOpen, setFiltersOpen] = useState(false);
    const [roleFilter, setRoleFilter] = useState('');
    const [roleFilterDraft, setRoleFilterDraft] = useState('');
    const [scopeTypeFilter, setScopeTypeFilter] = useState('');
    const [scopeTypeFilterDraft, setScopeTypeFilterDraft] = useState('');
    const [environmentFilter, setEnvironmentFilter] = useState('');
    const [environmentFilterDraft, setEnvironmentFilterDraft] = useState('');
    const normalizedQuickSearch = quickSearch.trim().toLowerCase();
    const bindingScopeType = Form.useWatch('scope_type', {
        form: rbac.bindingForm,
        preserve: true,
    });
    const bindingRoleID = Form.useWatch('role_id', {
        form: rbac.bindingForm,
        preserve: true,
    });
    const permissionCatalogMetadata = useMemo(
        () => new Map(
            rbac.permissions.map((permission) => {
                const catalogKey = permissionCatalogTranslationKey(permission.key);
                return [
                    permission.key,
                    {
                        label: t(`rbac.permissions.catalog.${catalogKey}.label`, {
                            defaultValue: permission.description?.trim() || permission.key,
                        }),
                        description: t(`rbac.permissions.catalog.${catalogKey}.description`, {
                            defaultValue: permission.description?.trim() || EMPTY_VALUE,
                        }),
                    },
                ];
            })
        ),
        [rbac.permissions, t]
    );
    const permissionDescriptionByKey = useMemo(
        () => new Map(Array.from(permissionCatalogMetadata.entries()).map(([key, value]) => [key, value.label])),
        [permissionCatalogMetadata]
    );
    const roleCatalogMetadata = useMemo(
        () => new Map(
            rbac.roles.map((role) => {
                return [
                    role.id,
                    {
                        label: localizeRoleLabel(t, role),
                        description: localizeRoleDescription(t, role) || EMPTY_VALUE,
                        assignment: localizeRoleAssignmentPolicy(t, role),
                    },
                ];
            })
        ),
        [rbac.roles, t]
    );
    const roleDisplayById = useMemo(
        () => new Map(rbac.roles.map((role) => [role.id, roleCatalogMetadata.get(role.id)?.label || role.display_name?.trim() || role.name])),
        [rbac.roles, roleCatalogMetadata]
    );
    const elevatedRoles = useMemo(
        () => rbac.roles.filter((role) => isPrivilegedRole(role)),
        [rbac.roles],
    );
    const elevatedRoleCatalogById = useMemo(
        () => new Map(elevatedRoles.map((role) => [role.id, role] as const)),
        [elevatedRoles],
    );
    const selectedBindingRole = useMemo(
        () => elevatedRoles.find((role) => role.id === bindingRoleID),
        [bindingRoleID, elevatedRoles]
    );
    const filteredRoles = useMemo(
        () =>
            rbac.roles.filter((role) => {
                if (roleFilter && role.id !== roleFilter) {
                    return false;
                }
                if (!normalizedQuickSearch) {
                    return true;
                }
                return [
                    role.name,
                    role.display_name,
                    role.description,
                    ...(role.permissions ?? []),
                ]
                    .filter(Boolean)
                    .join(' ')
                    .toLowerCase()
                    .includes(normalizedQuickSearch);
            }),
        [normalizedQuickSearch, rbac.roles, roleFilter],
    );
    const filteredRoleBindings = useMemo(
        () =>
            rbac.roleBindings.filter((binding) => {
                if (!isPrivilegedRoleBinding(binding, elevatedRoleCatalogById)) {
                    return false;
                }
                if (roleFilter && binding.role_id !== roleFilter) {
                    return false;
                }
                if (scopeTypeFilter && binding.scope_type !== scopeTypeFilter) {
                    return false;
                }
                if (environmentFilter && !(binding.allowed_environments ?? []).includes(environmentFilter as 'test' | 'prod')) {
                    return false;
                }
                if (!normalizedQuickSearch) {
                    return true;
                }
                return [
                    binding.role_name,
                    binding.role_display_name,
                    binding.scope_type,
                    binding.scope_display_name,
                    binding.scope_id,
                    ...(binding.allowed_environments ?? []),
                ]
                    .filter(Boolean)
                    .join(' ')
                    .toLowerCase()
                    .includes(normalizedQuickSearch);
            }),
        [elevatedRoleCatalogById, environmentFilter, normalizedQuickSearch, rbac.roleBindings, roleFilter, scopeTypeFilter],
    );
    const filteredPermissions = useMemo(
        () =>
            rbac.permissions.filter((permission) => {
                if (roleFilter) {
                    const selectedRole = rbac.roles.find((role) => role.id === roleFilter);
                    if (selectedRole && !(selectedRole.permissions ?? []).includes(permission.key)) {
                        return false;
                    }
                }
                if (!normalizedQuickSearch) {
                    return true;
                }
                return [permission.key, permission.description]
                    .filter(Boolean)
                    .join(' ')
                    .toLowerCase()
                    .includes(normalizedQuickSearch);
            }),
        [normalizedQuickSearch, rbac.permissions, rbac.roles, roleFilter],
    );
    const customRoleCount = filteredRoles.filter((role) => !role.built_in).length;

    const renderRoleIdentity = (role: Role) => {
        const primary = roleCatalogMetadata.get(role.id)?.label || role.display_name?.trim() || role.name;
        const showSecondary = primary !== role.name;
        return (
            <Space direction="vertical" size={0}>
                <Text strong>{primary}</Text>
                {showSecondary ? <Text type="secondary" style={{ fontSize: 13 }}>{role.name}</Text> : null}
            </Space>
        );
    };

    const renderBindingScope = (binding: GlobalRoleBinding) => {
        const scopeLabel = t(`rbac.scope.${binding.scope_type}`, { defaultValue: binding.scope_type });
        const scopeDisplay = binding.scope_display_name || binding.scope_id || t('rbac.bindings.global_scope');
        return (
            <Space direction="vertical" size={0}>
                <Tag>{scopeLabel}</Tag>
                <Text type="secondary" style={{ fontSize: 13 }}>{scopeDisplay}</Text>
            </Space>
        );
    };

    const roleColumns: ColumnsType<Role> = [
        {
            title: t('common:table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (_, role: Role) => renderRoleIdentity(role),
        },
        {
            title: t('common:table.description'),
            dataIndex: 'description',
            key: 'description',
            render: (_description: string | undefined, role: Role) => {
                const metadata = roleCatalogMetadata.get(role.id);
                const assignment = metadata?.assignment?.trim();
                return (
                    <Space direction="vertical" size={2}>
                        <Text>{metadata?.description || EMPTY_VALUE}</Text>
                        {assignment ? (
                            <Text type="secondary" style={{ fontSize: 13 }}>
                                {assignment}
                            </Text>
                        ) : null}
                    </Space>
                );
            },
        },
        {
            title: t('rbac.roles.permissions'),
            dataIndex: 'permissions',
            key: 'permissions',
            width: 420,
            render: (permissions: string[]) => (
                <Space wrap size={[6, 6]}>
                    {(permissions || []).map((key) => (
                        <Tag key={key} color="processing" title={key}>
                            {permissionDescriptionByKey.get(key) || key}
                        </Tag>
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
            render: (_, role: Role) => canManageRbac ? (
                <Space size={4} wrap>
                    <Button
                        type="link"
                        size="small"
                        icon={<EditOutlined />}
                        disabled={role.built_in}
                        data-testid={`rbac-role-action-edit-${role.id}`}
                        onClick={() => rbac.openEditRoleModal(role)}
                    >
                        {t('common:button.edit')}
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        disabled={role.built_in}
                        data-testid={`rbac-role-action-delete-${role.id}`}
                        onClick={() => rbac.openDeleteRoleModal(role)}
                    >
                        {t('common:button.delete')}
                    </Button>
                </Space>
            ) : null,
        },
    ];

    const bindingColumns: ColumnsType<GlobalRoleBinding> = [
        {
            title: t('rbac.bindings.role'),
            dataIndex: 'role_name',
            key: 'role_name',
            render: (roleName: string, record: GlobalRoleBinding) => {
                const localizedRoleLabel = roleDisplayById.get(record.role_id) || record.role_display_name || roleName || record.role_id;
                const role = elevatedRoleCatalogById.get(record.role_id);
                return (
                    <Space direction="vertical" size={0}>
                        <Tag color={getRoleAccessTagColor(role)}>
                            {localizedRoleLabel}
                        </Tag>
                        {localizedRoleLabel !== (roleName || record.role_id) ? (
                            <Text type="secondary" style={{ fontSize: 13 }}>{roleName || record.role_id}</Text>
                        ) : null}
                    </Space>
                );
            },
        },
        {
            title: t('rbac.bindings.scope'),
            key: 'scope',
            render: (_: unknown, record: GlobalRoleBinding) => renderBindingScope(record),
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
                        : <Tag color="gold">{t('rbac.bindings.all_environments')}</Tag>}
                </Space>
            ),
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 170,
            render: (value?: string) => <LocalDateTimeText value={value} />,
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 120,
            render: (_, binding: GlobalRoleBinding) => canManageRbac ? (
                <Popconfirm
                    title={t('rbac.bindings.delete_confirm')}
                    onConfirm={() => rbac.deleteRoleBinding(binding.id)}
                    okText={t('common:button.confirm')}
                    cancelText={t('common:button.cancel')}
                >
                    <Button
                        type="link"
                        danger
                        size="small"
                        data-testid={`rbac-binding-action-delete-${binding.id}`}
                        loading={rbac.deleteBindingPending && rbac.deletingBindingId === binding.id}
                    >
                        {t('common:button.delete')}
                    </Button>
                </Popconfirm>
            ) : null,
        },
    ];

    const permissionColumns: ColumnsType<Permission> = [
        {
            title: t('common:table.name'),
            dataIndex: 'key',
            key: 'key',
            render: (key: string, permission: Permission) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{permissionCatalogMetadata.get(permission.key)?.label || permission.description || key}</Text>
                    <Text type="secondary" style={{ fontSize: 13 }}>{key}</Text>
                </Space>
            ),
        },
        {
            title: t('common:table.description'),
            dataIndex: 'description',
            key: 'description',
            render: (_description: string | undefined, permission: Permission) => permissionCatalogMetadata.get(permission.key)?.description || EMPTY_VALUE,
        },
    ];

    const roleOptions = useMemo(
        () => rbac.roles.map((role) => ({
            value: role.id,
            label: roleDisplayById.get(role.id) || role.display_name || role.name,
        })),
        [rbac.roles, roleDisplayById]
    );
    const elevatedRoleOptions = useMemo(
        () => elevatedRoles.map((role) => ({
            value: role.id,
            label: roleDisplayById.get(role.id) || role.display_name || role.name,
        })),
        [elevatedRoles, roleDisplayById],
    );
    const localizedPermissionOptions = useMemo(
        () => rbac.permissions.map((permission) => {
            const metadata = permissionCatalogMetadata.get(permission.key);
            return {
                value: permission.key,
                label: metadata ? `${metadata.label} (${permission.key})` : permission.key,
            };
        }),
        [permissionCatalogMetadata, rbac.permissions]
    );
    const scopeOptions = useMemo(
        () => RBAC_SCOPE_VALUES.map((scope) => ({
            value: scope,
            label: t(`rbac.scope.${scope}`),
        })),
        [t]
    );
    const scopeTargetOptions = (rbac.scopeTargetOptionsByType?.[bindingScopeType || 'global'] ?? []) as ScopeTargetOption[];
    const scopeTargetLoading = Boolean(rbac.scopeTargetLoadingByType?.[bindingScopeType || 'global']);
    const environmentOptions = useMemo(
        () => ENVIRONMENT_VALUES.map((env) => ({
            value: env,
            label: t(`rbac.env.${env}`),
        })),
        [t]
    );

    useEffect(() => {
        const userId = searchParams.get('user_id')?.trim() ?? '';
        if (!userId || userId === selectedUserId) {
            return;
        }
        const userLabel = searchParams.get('user_label')?.trim() ?? '';
        selectUser(userId, userLabel);
    }, [searchParams, selectUser, selectedUserId]);

    return (
        <div data-testid="admin-rbac-page" className="admin-rbac-page">
            {rbac.messageContextHolder}
            <PageHeader
                title={t('rbac.title')}
                subtitle={t('rbac.subtitle')}
            />
            <PageSurface className="admin-rbac-page__filters-surface" style={{ marginBottom: 16 }}>
                <Alert
                    showIcon
                    type="info"
                    style={{ marginBottom: 16 }}
                    message={t('rbac.bindings.help_title')}
                    description={t('rbac.bindings.help_description')}
                />
                <PageSearchToolbar
                    searchValue={quickSearch}
                    searchDraftValue={quickSearchDraft}
                    onSearchDraftChange={setQuickSearchDraft}
                    onSearchChange={(value) => {
                        setQuickSearchDraft(value);
                        setQuickSearch(value);
                    }}
                    searchPlaceholder={t('rbac.search_placeholder', { defaultValue: 'Search roles, bindings, or permissions' })}
                    searchTestId="rbac-quick-search"
                    searchHelp={t('rbac.search_help', { defaultValue: 'Press Enter or click Search. Quick search filters the role catalog, elevated bindings, and permission directory on this page.' })}
                    advancedSearch={{
                        open: filtersOpen,
                        onToggle: () => setFiltersOpen((open) => !open),
                        openLabel: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                        closeLabel: t('common:search.hide_advanced', { defaultValue: 'Hide advanced search' }),
                        title: t('rbac.advanced_search_title', { defaultValue: 'Advanced search' }),
                        content: (
                            <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                <Text type="secondary">
                                    {t('rbac.advanced_search_help', {
                                        defaultValue: 'Choose exact RBAC filters here. Options can be searched by keyword, but the applied filter remains an exact value.',
                                    })}
                                </Text>
                                <Space wrap>
                                    <Select
                                        allowClear
                                        showSearch
                                        optionFilterProp="label"
                                        style={{ minWidth: 240 }}
                                        placeholder={t('rbac.bindings.role', { defaultValue: 'Role' })}
                                        value={roleFilterDraft || undefined}
                                        options={roleOptions}
                                        onChange={(value) => setRoleFilterDraft((value as string | undefined) ?? '')}
                                    />
                                    <Select
                                        allowClear
                                        showSearch
                                        optionFilterProp="label"
                                        style={{ minWidth: 180 }}
                                        placeholder={t('rbac.bindings.scope_type', { defaultValue: 'Scope type' })}
                                        value={scopeTypeFilterDraft || undefined}
                                        options={scopeOptions}
                                        onChange={(value) => setScopeTypeFilterDraft((value as string | undefined) ?? '')}
                                    />
                                    <Select
                                        allowClear
                                        showSearch
                                        optionFilterProp="label"
                                        style={{ minWidth: 180 }}
                                        placeholder={t('rbac.bindings.allowed_envs', { defaultValue: 'Allowed environments' })}
                                        value={environmentFilterDraft || undefined}
                                        options={environmentOptions}
                                        onChange={(value) => setEnvironmentFilterDraft((value as string | undefined) ?? '')}
                                    />
                                    <Button
                                        type="primary"
                                        onClick={() => {
                                            setQuickSearch(quickSearchDraft);
                                            setRoleFilter(roleFilterDraft);
                                            setScopeTypeFilter(scopeTypeFilterDraft);
                                            setEnvironmentFilter(environmentFilterDraft);
                                        }}
                                    >
                                        {t('common:button.search')}
                                    </Button>
                                </Space>
                            </Space>
                        ),
                    }}
                    hasActiveFilters={
                        quickSearch.trim().length > 0 ||
                        roleFilter.length > 0 ||
                        scopeTypeFilter.length > 0 ||
                        environmentFilter.length > 0
                    }
                    onClear={() => {
                        setQuickSearch('');
                        setQuickSearchDraft('');
                        setRoleFilter('');
                        setRoleFilterDraft('');
                        setScopeTypeFilter('');
                        setScopeTypeFilterDraft('');
                        setEnvironmentFilter('');
                        setEnvironmentFilterDraft('');
                    }}
                    clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                />
            </PageSurface>

            <div className="summary-card-grid admin-rbac-page__summary-grid">
                <SummaryMetricCard
                    title={t('rbac.summary.roles_title')}
                    value={filteredRoles.length}
                    description={t('rbac.summary.roles_description')}
                    visual={<RoleCatalogGlyph className="summary-metric-card__art" />}
                    accentColor="#D97706"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('rbac.summary.custom_roles_title')}
                    value={customRoleCount}
                    description={t('rbac.summary.custom_roles_description')}
                    visual={<AccessControlGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('rbac.summary.bindings_title')}
                    value={filteredRoleBindings.length}
                    description={rbac.selectedUserDisplayLabel
                        ? t('rbac.summary.bindings_description_selected', { user: rbac.selectedUserDisplayLabel })
                        : t('rbac.summary.bindings_description')}
                    visual={<SafetyCertificateOutlined className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
                <SummaryMetricCard
                    title={t('rbac.summary.permissions_title')}
                    value={filteredPermissions.length}
                    description={t('rbac.summary.permissions_description')}
                    visual={<UserDirectoryGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface className="admin-rbac-page__roles-surface" style={{ marginBottom: 16 }}>
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
                        {canManageRbac ? (
                            <Button type="primary" icon={<PlusOutlined />} data-testid="rbac-role-create-button" onClick={rbac.openCreateRoleModal}>
                                {t('rbac.roles.add')}
                            </Button>
                        ) : null}
                    </Space>
                </Space>
                <Alert
                    showIcon
                    type="info"
                    style={{ marginTop: 16 }}
                    message={t('rbac.roles.help_title')}
                    description={t('rbac.roles.help_description')}
                />

                <Table<Role>
                    style={{ marginTop: 16 }}
                    rowKey="id"
                    columns={roleColumns}
                    dataSource={filteredRoles}
                    loading={rbac.rolesLoading}
                    locale={{
                        emptyText: (
                            <div style={{ padding: 40 }}>
                                <ActionEmptyState
                                    title={t('rbac.roles.empty')}
                                    description={t('rbac.roles.empty_description')}
                                    visual={<RoleCatalogGlyph className="action-empty-state__art" />}
                                    actions={canManageRbac ? (
                                        <Button type="primary" icon={<PlusOutlined />} onClick={rbac.openCreateRoleModal}>
                                            {t('rbac.roles.add')}
                                        </Button>
                                    ) : undefined}
                                />
                            </div>
                        ),
                    }}
                    pagination={false}
                />
            </PageSurface>

            <PageSurface className="admin-rbac-page__bindings-surface" style={{ marginBottom: 16 }}>
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
                        {canManageRbac ? (
                            <Button type="primary" icon={<PlusOutlined />} data-testid="rbac-binding-create-button" onClick={() => rbac.openAddBindingModal()}>
                                {t('rbac.bindings.add')}
                            </Button>
                        ) : null}
                    </Space>
                </Space>
                <Alert
                    showIcon
                    type="info"
                    style={{ marginTop: 16, marginBottom: 16 }}
                    message={t('rbac.bindings.help_title')}
                    description={t('rbac.bindings.help_description')}
                />

                <Space align="center" className="admin-rbac-page__binding-toolbar" style={{ marginTop: 16, marginBottom: 16 }}>
                    <SafetyCertificateOutlined />
                    <Text>{t('rbac.bindings.select_user')}</Text>
                    <Select
                        allowClear
                        showSearch
                        filterOption={false}
                        style={{ minWidth: 320 }}
                        value={rbac.selectedUserValue}
                        loading={rbac.usersLoading}
                        placeholder={t('rbac.bindings.select_user_placeholder')}
                        data-testid="rbac-user-selector"
                        searchValue={rbac.userSearch}
                        onSearch={rbac.setUserSearch}
                        onChange={(value, option) => {
                            if (!value) {
                                rbac.selectUser('', '');
                                return;
                            }
                            const resolvedLabel =
                                !Array.isArray(option) && option && typeof option === 'object' && 'label' in option && typeof option.label === 'string'
                                    ? option.label
                                    : '';
                            rbac.selectUser(String(value), resolvedLabel);
                        }}
                        options={rbac.userOptions}
                        notFoundContent={
                            rbac.usersLoading
                                ? t('common:message.loading')
                                : rbac.userSearch.trim()
                                    ? t('rbac.bindings.no_matching_users')
                                    : t('common:message.no_data')
                        }
                    />
                </Space>

                {rbac.selectedUserId ? (
                    <div className="admin-rbac-page__binding-selection" style={{ marginBottom: 16 }}>
                        <Text strong>{rbac.selectedUserDisplayLabel}</Text>
                        <br />
                        <Text type="secondary">{t('rbac.bindings.selected_user_hint')}</Text>
                    </div>
                ) : null}

                <Table<GlobalRoleBinding>
                    rowKey="id"
                    columns={bindingColumns}
                    dataSource={filteredRoleBindings}
                    loading={rbac.roleBindingsLoading}
                    locale={{
                        emptyText: rbac.selectedUserId
                            ? (
                                <div style={{ padding: 40 }}>
                                    <ActionEmptyState
                                        compact={true}
                                        title={t('rbac.bindings.empty')}
                                        description={t('rbac.bindings.empty_description', { user: rbac.selectedUserDisplayLabel || EMPTY_VALUE })}
                                        visual={<AccessControlGlyph className="action-empty-state__art" />}
                                        actions={canManageRbac ? (
                                            <Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => rbac.openAddBindingModal()}>
                                                {t('rbac.bindings.add')}
                                            </Button>
                                        ) : undefined}
                                    />
                                </div>
                            )
                            : (
                                <div style={{ padding: 40 }}>
                                    <ActionEmptyState
                                        compact={true}
                                        title={t('rbac.bindings.select_user_first')}
                                        description={t('rbac.bindings.select_user_help')}
                                        visual={<UserDirectoryGlyph className="action-empty-state__art" />}
                                    />
                                </div>
                            ),
                    }}
                    pagination={false}
                />
            </PageSurface>

            <PageSurface className="admin-rbac-page__permissions-surface">
                <Space direction="vertical" size={0} style={{ marginBottom: 16 }}>
                    <Text strong>{t('rbac.permissions.title')}</Text>
                    <Text type="secondary">{t('rbac.permissions.subtitle')}</Text>
                </Space>
                <Alert
                    showIcon
                    type="info"
                    style={{ marginBottom: 16 }}
                    message={t('rbac.permissions.help_title')}
                    description={t('rbac.permissions.help_description')}
                />
                <Table<Permission>
                    rowKey="key"
                    columns={permissionColumns}
                    dataSource={filteredPermissions}
                    loading={rbac.permissionsLoading}
                    locale={{
                        emptyText: (
                            <div style={{ padding: 40 }}>
                                <ActionEmptyState
                                    compact={true}
                                    title={t('rbac.permissions.empty')}
                                    description={t('rbac.permissions.empty_description')}
                                    visual={<AccessControlGlyph className="action-empty-state__art" />}
                                />
                            </div>
                        ),
                    }}
                    pagination={false}
                />
            </PageSurface>

            {rbac.createRoleOpen ? (
                <Modal
                    title={t('rbac.roles.add_title')}
                    open={rbac.createRoleOpen}
                    onOk={() => {
                        void rbac.submitCreateRole();
                    }}
                    onCancel={rbac.closeCreateRoleModal}
                    confirmLoading={rbac.createRolePending}
                    maskClosable={false}
                    keyboard={false}
                    data-testid="rbac-role-create-modal"
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
                            <Select mode="multiple" options={localizedPermissionOptions} optionFilterProp="label" />
                        </Form.Item>
                        <Form.Item name="enabled" label={t('common:table.status')} valuePropName="checked" initialValue={true}>
                            <Switch />
                        </Form.Item>
                    </Form>
                </Modal>
            ) : null}

            {rbac.editRoleOpen ? (
                <Modal
                    title={t('rbac.roles.edit_title', { name: rbac.editingRole?.display_name || rbac.editingRole?.name || '' })}
                    open={rbac.editRoleOpen}
                    onOk={() => {
                        void rbac.submitEditRole();
                    }}
                    onCancel={rbac.closeEditRoleModal}
                    confirmLoading={rbac.updateRolePending}
                    maskClosable={false}
                    keyboard={false}
                    data-testid="rbac-role-edit-modal"
                >
                    <Form form={rbac.roleEditForm} layout="vertical" preserve={false}>
                        <Form.Item name="display_name" label={t('common:table.display_name')}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="description" label={t('common:table.description')}>
                            <Input.TextArea rows={3} />
                        </Form.Item>
                        <Form.Item name="permissions" label={t('rbac.roles.permissions')} rules={[{ required: true }]}>
                            <Select mode="multiple" options={localizedPermissionOptions} optionFilterProp="label" />
                        </Form.Item>
                        <Form.Item name="enabled" label={t('common:table.status')} valuePropName="checked">
                            <Switch />
                        </Form.Item>
                    </Form>
                </Modal>
            ) : null}

            <Modal
                title={t('rbac.roles.delete_title')}
                open={rbac.deleteRoleOpen}
                onOk={rbac.submitDeleteRole}
                onCancel={rbac.closeDeleteRoleModal}
                confirmLoading={rbac.deleteRolePending}
                okButtonProps={{ danger: true }}
            >
                <Text>{t('rbac.roles.delete_confirm', { name: rbac.deletingRole?.display_name || rbac.deletingRole?.name || '' })}</Text>
            </Modal>

            {rbac.addBindingOpen ? (
                <Modal
                    title={t('rbac.bindings.add_title')}
                    open={rbac.addBindingOpen}
                    onOk={() => {
                        void rbac.submitAddBinding();
                    }}
                    onCancel={rbac.closeAddBindingModal}
                    confirmLoading={rbac.createBindingPending}
                    maskClosable={false}
                    keyboard={false}
                    data-testid="rbac-binding-add-modal"
                >
                    <Form form={rbac.bindingForm} layout="vertical" preserve={false}>
                        <Form.Item label={t('rbac.bindings.select_user')}>
                            <Input value={rbac.selectedUserDisplayLabel} readOnly />
                        </Form.Item>
                        <Form.Item name="role_id" label={t('rbac.bindings.role')} rules={[{ required: true }]}>
                            <Select options={elevatedRoleOptions} optionFilterProp="label" showSearch />
                        </Form.Item>
                        {selectedBindingRole ? (
                            <Alert
                                showIcon
                                type="warning"
                                style={{ marginBottom: 16 }}
                                message={t('rbac.bindings.role_policy_title')}
                                description={
                                    localizeRoleAssignmentPolicy(t, selectedBindingRole)
                                    || localizeRoleDescription(t, selectedBindingRole)
                                }
                            />
                        ) : null}
                        <Form.Item name="scope_type" label={t('rbac.bindings.scope_type')} rules={[{ required: true }]} initialValue="global">
                            <Select
                                options={scopeOptions}
                                onChange={(value) => {
                                    rbac.bindingForm.setFieldsValue({ scope_type: value, scope_id: undefined });
                                }}
                            />
                        </Form.Item>
                        {bindingScopeType && bindingScopeType !== 'global' ? (
                            <Form.Item
                                name="scope_id"
                                label={t('rbac.bindings.scope_id')}
                                extra={t('rbac.bindings.scope_id_help', {
                                    scope: t(`rbac.scope.${bindingScopeType}`, { defaultValue: bindingScopeType }),
                                })}
                            >
                                <AutoComplete
                                    options={scopeTargetOptions}
                                    allowClear={true}
                                    placeholder={t('rbac.bindings.scope_id_placeholder')}
                                    filterOption={(inputValue, option) => {
                                        const label = String(option?.label ?? '').toLowerCase();
                                        const value = String(option?.value ?? '').toLowerCase();
                                        const search = inputValue.trim().toLowerCase();
                                        return label.includes(search) || value.includes(search);
                                    }}
                                    notFoundContent={scopeTargetLoading
                                        ? t('common:status.loading', { defaultValue: 'Loading…' })
                                        : t('rbac.bindings.scope_target_empty', { defaultValue: 'No suggested targets yet' })}
                                />
                            </Form.Item>
                        ) : null}
                        <Form.Item
                            name="allowed_environments"
                            label={t('rbac.bindings.allowed_envs')}
                            extra={t('rbac.bindings.allowed_envs_help')}
                        >
                            <Select mode="multiple" options={environmentOptions} />
                        </Form.Item>
                    </Form>
                </Modal>
            ) : null}
        </div>
    );
}
