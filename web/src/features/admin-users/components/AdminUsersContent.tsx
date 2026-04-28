'use client';

import {
    Alert,
    AutoComplete,
    Button,
    Card,
    Drawer,
    Form,
    Input,
    List,
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
import type { ColumnsType } from 'antd/es/table';
import {
    DeleteOutlined,
    DownOutlined,
    EditOutlined,
    PlusOutlined,
    ReloadOutlined,
    SafetyCertificateOutlined,
    SettingOutlined,
    UpOutlined,
} from '@ant-design/icons';
import { useRouter } from 'next/navigation';
import { useEffect, useMemo, useState, type Key } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    localizeRoleAssignmentPolicy,
    localizeRoleDescription,
    localizeRoleLabel,
} from '@/features/rbac-shared/roleCatalogI18n';
import {
    AccessControlGlyph,
    QueueReviewGlyph,
    RoleCatalogGlyph,
    UserDirectoryGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { useDisplayTimeZone } from '@/components/providers/DisplayTimeZoneProvider';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import { useUserPreference } from '@/hooks/useUserPreference';
import { hasAnyPermission, hasPermission } from '@/lib/auth/permissions';
import { useAuthStore } from '@/stores/auth';
import {
    getRoleAccessTagColor,
    isPrivilegedRole,
    isPrivilegedRoleBinding,
} from '@/features/rbac-shared/privilegedAccess';
import { UserDirectorySelectionPanel } from '@/features/rbac-shared/UserDirectorySelectionPanel';
import { useUserRoleBindingsManager } from '@/features/rbac-shared/useUserRoleBindingsManager';
import {
    buildDefaultUserTableColumnKeys,
    buildOrderedUserTableDisplayColumns,
    buildUserTableColumnOptions,
    normalizeUserTableMergedColumns,
    normalizeUserTablePreferenceColumns,
    stringifyUserProfileAttributeValue,
    type NormalizedUserTableMergedColumn,
    type UserTableColumnOption,
    type UserTablePreferenceValue,
    USER_DIRECTORY_DISPLAY_PREFERENCE_KEY,
} from '@/features/rbac-shared/userDirectoryDisplayConfig';
import { useAdminUsersController } from '../hooks/useAdminUsersController';
import {
    type User,
} from '../types';
import { ENVIRONMENT_VALUES, RBAC_SCOPE_VALUES, type GlobalRoleBinding } from '@/features/admin-rbac/types';

const { Text } = Typography;
const EMPTY_VALUE = '—';
const USER_TABLE_FIXED_IDENTITY_COLUMN_KEY = 'identity';
const USER_TABLE_FIXED_ACTIONS_COLUMN_KEY = 'actions';

export {
    buildOrderedUserTableDisplayColumns,
    buildUserTableColumnOptions,
    normalizeUserTableMergedColumns,
    normalizeUserTablePreferenceColumns,
} from '@/features/rbac-shared/userDirectoryDisplayConfig';

interface UserSearchFieldOption {
    value: string;
    label: string;
}

interface UserSearchValueOption {
    value: string;
    label: string;
}

interface AdvancedUserSearchCondition {
    field: string;
    value: string;
}

interface UserTableMergedColumnDraft {
    id: string;
    label: string;
    columnKeys: string[];
    showLabels: boolean;
}

let mergedColumnDraftCounter = 0;

function createMergedColumnDraftId() {
    mergedColumnDraftCounter += 1;
    return `merged-column-${mergedColumnDraftCounter}`;
}

function quoteAdminUserSearchValue(value: string) {
    const trimmed = value.trim();
    if (!trimmed) {
        return '';
    }
    if (!/[\s"]/u.test(trimmed)) {
        return trimmed;
    }
    return `"${trimmed.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

function buildAdminUserSearchQuery(
    quickSearch: string,
    conditions: AdvancedUserSearchCondition[],
) {
    const terms: string[] = [];
    const quick = quickSearch.trim();
    if (quick) {
        terms.push(quick);
    }
    for (const condition of conditions) {
        const field = condition.field.trim();
        const value = quoteAdminUserSearchValue(condition.value);
        if (!field || !value) {
            continue;
        }
        terms.push(`${field}:${value}`);
    }
    return terms.join(' ').trim();
}

function normalizeAdvancedUserSearchConditions(conditions: AdvancedUserSearchCondition[]) {
    return conditions
        .map((condition) => ({
            field: condition.field.trim(),
            value: condition.value.trim(),
        }))
        .filter((condition) => condition.field && condition.value);
}

function dedupeUserSearchValueOptions(options: UserSearchValueOption[]) {
    const seen = new Set<string>();
    const normalized: UserSearchValueOption[] = [];
    for (const option of options) {
        const value = option.value.trim();
        if (!value) {
            continue;
        }
        const key = value.toLowerCase();
        if (seen.has(key)) {
            continue;
        }
        seen.add(key);
        normalized.push({
            value,
            label: option.label || value,
        });
    }
    return normalized.sort((left, right) => left.label.localeCompare(right.label));
}

function formatAdminUsersLocalDate(value?: string | null, timeZone?: string | null) {
    if (!value || value.trim() === '') {
        return null;
    }

    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
        return value;
    }

    const formatter = new Intl.DateTimeFormat(undefined, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        ...(timeZone ? { timeZone } : {}),
    });
    const parts = formatter.formatToParts(parsed);
    const pick = (type: Intl.DateTimeFormatPartTypes) =>
        parts.find((part) => part.type === type)?.value ?? '';
    return `${pick('year')}-${pick('month')}-${pick('day')}`;
}

function formatAdminUsersLocalDateTime(value?: string | null, timeZone?: string | null) {
    if (!value || value.trim() === '') {
        return null;
    }

    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
        return value;
    }

    const formatter = new Intl.DateTimeFormat(undefined, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hourCycle: 'h23',
        ...(timeZone ? { timeZone } : {}),
    });
    const parts = formatter.formatToParts(parsed);
    const pick = (type: Intl.DateTimeFormatPartTypes) =>
        parts.find((part) => part.type === type)?.value ?? '';
    return `${pick('year')}-${pick('month')}-${pick('day')} ${pick('hour')}:${pick('minute')}`;
}

function buildMergedColumnDrafts(groups: NormalizedUserTableMergedColumn[]): UserTableMergedColumnDraft[] {
    return groups.map((group) => ({
        id: createMergedColumnDraftId(),
        label: group.label ?? '',
        columnKeys: [...group.columnKeys],
        showLabels: group.showLabels,
    }));
}

function estimateUsersTableScrollWidth(visibleConfigurableColumnCount: number) {
    return 460 + visibleConfigurableColumnCount * 190;
}

export function AdminUsersContent() {
    const { t } = useTranslation(['admin', 'common']);
    const { displayTimeZone } = useDisplayTimeZone();
    const router = useRouter();
    const currentUser = useAuthStore((state) => state.user);
    const canManageUsers = hasPermission(currentUser, 'user:manage');
    const canReadUserBindings = hasAnyPermission(currentUser, ['rbac:read', 'rbac:manage']);
    const canManageUserBindings = hasPermission(currentUser, 'rbac:manage');
    const users = useAdminUsersController({ t });
    const { setPage, setSearch } = users;
    const [selectedAccessUser, setSelectedAccessUser] = useState<Pick<User, 'id' | 'username' | 'display_name'> | null>(null);
    const [selectedDirectoryUserIds, setSelectedDirectoryUserIds] = useState<string[]>([]);
    const [selectedDirectoryUserRecords, setSelectedDirectoryUserRecords] = useState<Record<string, User>>({});
    const [selectedRoleBindingIds, setSelectedRoleBindingIds] = useState<string[]>([]);
    const [quickSearch, setQuickSearch] = useState('');
    const [quickSearchDraft, setQuickSearchDraft] = useState('');
    const [advancedSearchOpen, setAdvancedSearchOpen] = useState(false);
    const [advancedSearchConditions, setAdvancedSearchConditions] = useState<AdvancedUserSearchCondition[]>([]);
    const [advancedSearchDraftConditions, setAdvancedSearchDraftConditions] = useState<AdvancedUserSearchCondition[]>([]);
    const [columnsDrawerOpen, setColumnsDrawerOpen] = useState(false);
    const [columnDraftKeys, setColumnDraftKeys] = useState<string[]>([]);
    const [mergedColumnDrafts, setMergedColumnDrafts] = useState<UserTableMergedColumnDraft[]>([]);
    const userItems = useMemo(
        () => users.users?.items ?? [],
        [users.users?.items]
    );
    const roleCatalog = useMemo(
        () => users.roles?.items ?? [],
        [users.roles?.items]
    );
    const roleCatalogById = useMemo(
        () => new Map(roleCatalog.map((role) => [role.id, role])),
        [roleCatalog]
    );
    const roleCatalogByName = useMemo(
        () => new Map(roleCatalog.map((role) => [role.name, role])),
        [roleCatalog]
    );
    const roleDisplayByName = useMemo(
        () => new Map(roleCatalog.map((role) => [role.name, localizeRoleLabel(t, role)])),
        [roleCatalog, t]
    );
    const roleOptions = useMemo(
        () => roleCatalog.map((role) => ({
            value: role.id,
            label: localizeRoleLabel(t, role),
        })),
        [roleCatalog, t]
    );
    const totalUsers = users.users?.pagination?.total ?? userItems.length;
    const enabledUsers = userItems.filter((user) => user.enabled).length;
    const usersWithExplicitRoles = userItems.filter((user) => (user.roles ?? []).length > 0).length;
    const usersWithDirectoryProfiles = userItems.filter((user) => Object.keys(user.profile_attributes ?? {}).length > 0).length;
    const userProfileFields = useMemo(
        () => users.users?.profile_fields ?? [],
        [users.users?.profile_fields]
    );
    const userTableColumnOptions = useMemo(
        () => buildUserTableColumnOptions(t, userProfileFields),
        [t, userProfileFields]
    );
    const defaultUserTableColumnKeys = useMemo(
        () => buildDefaultUserTableColumnKeys(userProfileFields),
        [userProfileFields]
    );
    const userTablePreference = useUserPreference<UserTablePreferenceValue>(USER_DIRECTORY_DISPLAY_PREFERENCE_KEY);
    const selectedUserTableColumnKeys = useMemo(
        () =>
            normalizeUserTablePreferenceColumns(
                userTablePreference.value?.columns,
                userTableColumnOptions,
                defaultUserTableColumnKeys,
            ),
        [defaultUserTableColumnKeys, userTableColumnOptions, userTablePreference.value?.columns]
    );
    const selectedMergedColumns = useMemo(
        () =>
            normalizeUserTableMergedColumns(
                userTablePreference.value?.merged_columns,
                selectedUserTableColumnKeys,
                userTableColumnOptions,
                userTablePreference.value?.merged_column_keys,
                userTablePreference.value?.merged_column_label,
            ),
        [
            selectedUserTableColumnKeys,
            userTableColumnOptions,
            userTablePreference.value?.merged_column_keys,
            userTablePreference.value?.merged_column_label,
            userTablePreference.value?.merged_columns,
        ]
    );
    const userSearchFieldOptions = useMemo<UserSearchFieldOption[]>(
        () => [
            { value: 'username', label: t('users.search.field.username') },
            { value: 'display_name', label: t('users.search.field.display_name') },
            { value: 'email', label: t('users.search.field.email') },
            { value: 'role', label: t('users.search.field.role', { defaultValue: 'Role' }) },
            { value: 'status', label: t('users.search.field.status', { defaultValue: 'Status' }) },
            ...userProfileFields
                .filter((field) => field.searchable !== false)
                .map((field) => ({
                    value: field.key,
                    label: field.label,
                })),
        ],
        [t, userProfileFields]
    );
    const userSearchValueOptionsByField = useMemo(() => {
        const valueOptions = new Map<string, { kind: 'select' | 'suggest'; options: UserSearchValueOption[] }>();
        const statusOptions = dedupeUserSearchValueOptions([
            {
                value: 'enabled',
                label: t('users.status.enabled'),
            },
            {
                value: 'disabled',
                label: t('users.status.disabled'),
            },
        ]);
        valueOptions.set('status', { kind: 'select', options: statusOptions });
        valueOptions.set(
            'role',
            {
                kind: 'select',
                options: dedupeUserSearchValueOptions(
                    roleCatalog.map((role) => ({
                        value: role.display_name || role.name || role.id,
                        label: localizeRoleLabel(t, role),
                    })),
                ),
            },
        );
        valueOptions.set(
            'username',
            {
                kind: 'suggest',
                options: dedupeUserSearchValueOptions(
                    userItems
                        .filter((user) => Boolean(user.username))
                        .map((user) => ({
                            value: user.username,
                            label: user.display_name
                                ? `${user.display_name} (${user.username})`
                                : user.username,
                        })),
                ),
            },
        );
        valueOptions.set(
            'display_name',
            {
                kind: 'suggest',
                options: dedupeUserSearchValueOptions(
                    userItems
                        .filter((user) => Boolean(user.display_name))
                        .map((user) => ({
                            value: user.display_name as string,
                            label: `${user.display_name}${user.username ? ` (${user.username})` : ''}`,
                        })),
                ),
            },
        );
        valueOptions.set(
            'email',
            {
                kind: 'suggest',
                options: dedupeUserSearchValueOptions(
                    userItems
                        .filter((user) => Boolean(user.email))
                        .map((user) => ({
                            value: user.email as string,
                            label: user.email as string,
                        })),
                ),
            },
        );
        for (const field of userProfileFields) {
            if (field.searchable === false) {
                continue;
            }
            const options = dedupeUserSearchValueOptions(
                userItems
                    .map((user) => stringifyUserProfileAttributeValue(user.profile_attributes?.[field.key]))
                    .filter((value) => value !== EMPTY_VALUE)
                    .map((value) => ({
                        value,
                        label: value,
                    })),
            );
            valueOptions.set(field.key, {
                kind: 'suggest',
                options,
            });
        }
        return valueOptions;
    }, [roleCatalog, t, userItems, userProfileFields]);
    const combinedUserSearch = useMemo(
        () => buildAdminUserSearchQuery(quickSearch, advancedSearchConditions),
        [advancedSearchConditions, quickSearch]
    );
    const selectedAccessUserId = selectedAccessUser?.id ?? '';
    const accessBindings = useUserRoleBindingsManager({
        t,
        selectedUserId: selectedAccessUserId,
        messageApi: users.messageApi,
        enabled: Boolean(selectedAccessUserId),
    });
    const bindingScopeType = Form.useWatch('scope_type', {
        form: accessBindings.bindingForm,
        preserve: true,
    });
    const selectedBindingRoleID = Form.useWatch('role_id', {
        form: accessBindings.bindingForm,
        preserve: true,
    });
    const selectedBindingUserIds = accessBindings.effectiveSelectedBindingUserIds;
    const selectedBindingRole = useMemo(
        () => roleCatalog.find((role) => role.id === selectedBindingRoleID),
        [roleCatalog, selectedBindingRoleID]
    );
    const runtimeBindings = useMemo(
        () => accessBindings.roleBindings.filter((binding) => !isPrivilegedRoleBinding(binding, roleCatalogById)),
        [accessBindings.roleBindings, roleCatalogById]
    );
    const elevatedBindings = useMemo(
        () => accessBindings.roleBindings.filter((binding) => isPrivilegedRoleBinding(binding, roleCatalogById)),
        [accessBindings.roleBindings, roleCatalogById]
    );
    const directoryUserRowSelection = useMemo(
        () => ({
            selectedRowKeys: selectedDirectoryUserIds,
            preserveSelectedRowKeys: true,
            onChange: (selectedRowKeys: Key[], selectedRows: User[]) => {
                const nextIds = selectedRowKeys.map((value) => String(value));
                setSelectedDirectoryUserIds(nextIds);
                setSelectedDirectoryUserRecords((current) => {
                    const nextRecords: Record<string, User> = {};
                    for (const userId of nextIds) {
                        if (current[userId]) {
                            nextRecords[userId] = current[userId];
                        }
                    }
                    for (const row of selectedRows) {
                        nextRecords[row.id] = row;
                    }
                    return nextRecords;
                });
            },
        }),
        [selectedDirectoryUserIds],
    );
    const selectedDirectoryUsers = useMemo(
        () =>
            selectedDirectoryUserIds
                .map((userId) => selectedDirectoryUserRecords[userId])
                .filter((user): user is User => Boolean(user)),
        [selectedDirectoryUserIds, selectedDirectoryUserRecords],
    );
    const visibleSelectedRoleBindingIds = useMemo(() => {
        const validIds = new Set(accessBindings.roleBindings.map((binding) => binding.id));
        return selectedRoleBindingIds.filter((bindingId) => validIds.has(bindingId));
    }, [accessBindings.roleBindings, selectedRoleBindingIds]);
    const accessBindingRowSelection = useMemo(
        () => ({
            selectedRowKeys: visibleSelectedRoleBindingIds,
            onChange: (selectedRowKeys: Key[]) => {
                setSelectedRoleBindingIds(selectedRowKeys.map((value) => String(value)));
            },
        }),
        [visibleSelectedRoleBindingIds],
    );

    const toggleAdvancedSearch = () => {
        setAdvancedSearchOpen((open) => {
            const nextOpen = !open;
            if (nextOpen) {
                setAdvancedSearchDraftConditions(
                    advancedSearchConditions.length > 0
                        ? advancedSearchConditions
                        : [{ field: '', value: '' }]
                );
            }
            return nextOpen;
        });
    };

    useEffect(() => {
        setSearch(combinedUserSearch);
        setPage(1);
    }, [combinedUserSearch, setPage, setSearch]);

    const applyQuickSearch = (value = quickSearchDraft) => {
        setQuickSearchDraft(value);
        setQuickSearch(value);
    };

    const applyAdvancedSearch = () => {
        setQuickSearch(quickSearchDraft);
        setAdvancedSearchConditions(
            normalizeAdvancedUserSearchConditions(advancedSearchDraftConditions),
        );
    };

    const openColumnsDrawer = () => {
        setColumnDraftKeys(selectedUserTableColumnKeys);
        setMergedColumnDrafts(buildMergedColumnDrafts(selectedMergedColumns));
        setColumnsDrawerOpen(true);
    };

    const addDraftColumn = (columnKey: string) => {
        setColumnDraftKeys((current) => (current.includes(columnKey) ? current : [...current, columnKey]));
    };

    const removeDraftColumn = (columnKey: string) => {
        setColumnDraftKeys((current) => current.filter((key) => key !== columnKey));
        setMergedColumnDrafts((current) =>
            current.map((draft) => ({
                ...draft,
                columnKeys: draft.columnKeys.filter((key) => key !== columnKey),
            }))
        );
    };

    const moveDraftColumn = (columnKey: string, direction: 'up' | 'down') => {
        setColumnDraftKeys((current) => {
            const index = current.indexOf(columnKey);
            if (index < 0) {
                return current;
            }
            const nextIndex = direction === 'up' ? index - 1 : index + 1;
            if (nextIndex < 0 || nextIndex >= current.length) {
                return current;
            }
            const next = current.slice();
            [next[index], next[nextIndex]] = [next[nextIndex], next[index]];
            return next;
        });
    };

    const resetDraftColumns = () => {
        setColumnDraftKeys(defaultUserTableColumnKeys);
        setMergedColumnDrafts([]);
    };

    const addMergedColumnDraft = () => {
        setMergedColumnDrafts((current) => [
            ...current,
            { id: createMergedColumnDraftId(), label: '', columnKeys: [], showLabels: true },
        ]);
    };

    const updateMergedColumnDraft = (
        draftId: string,
        patch: Partial<Pick<UserTableMergedColumnDraft, 'label' | 'columnKeys' | 'showLabels'>>,
    ) => {
        setMergedColumnDrafts((current) =>
            current.map((draft) => (draft.id === draftId ? { ...draft, ...patch } : draft))
        );
    };

    const removeMergedColumnDraft = (draftId: string) => {
        setMergedColumnDrafts((current) => current.filter((draft) => draft.id !== draftId));
    };

    const saveColumnPreference = async () => {
        const normalized = normalizeUserTablePreferenceColumns(
            columnDraftKeys,
            userTableColumnOptions,
            defaultUserTableColumnKeys,
        );
        if (normalized.length === 0) {
            await userTablePreference.resetPreference();
            setColumnsDrawerOpen(false);
            return;
        }
        await userTablePreference.savePreference({
            value: {
                columns: normalized,
                merged_columns: normalizeUserTableMergedColumns(
                    mergedColumnDrafts.map((draft) => ({
                        label: draft.label,
                        column_keys: draft.columnKeys,
                        show_labels: draft.showLabels,
                    })),
                    normalized,
                    userTableColumnOptions,
                ).map((group) => ({
                    label: group.label,
                    column_keys: group.columnKeys,
                    show_labels: group.showLabels,
                })),
            },
        });
        setColumnsDrawerOpen(false);
    };

    const resetStoredColumnPreference = async () => {
        await userTablePreference.resetPreference();
        setColumnDraftKeys(defaultUserTableColumnKeys);
        setMergedColumnDrafts([]);
        setColumnsDrawerOpen(false);
    };

    const renderUserIdentity = (
        record: Pick<User, 'username' | 'display_name' | 'email' | 'id' | 'enabled'>
    ) => {
        const username = 'username' in record ? record.username : undefined;
        const displayName = 'display_name' in record ? record.display_name : undefined;
        const identityId = record.id;
        const primary = displayName?.trim() || username || identityId;
        const secondary = username && username !== primary ? username : identityId;
        const statusClassName = record.enabled
            ? 'admin-users-table__identity-primary--enabled'
            : 'admin-users-table__identity-primary--disabled';
        const statusDotClassName = record.enabled
            ? 'admin-users-table__identity-status-dot--enabled'
            : 'admin-users-table__identity-status-dot--disabled';

        return (
            <div className="admin-users-table__identity">
                <div className="admin-users-table__identity-primary-row">
                    <span
                        aria-hidden="true"
                        className={`admin-users-table__identity-status-dot ${statusDotClassName}`}
                    />
                    <Text strong className={`admin-users-table__identity-primary ${statusClassName}`}>
                        {primary}
                    </Text>
                </div>
                {secondary ? (
                    <Text type="secondary" className="admin-users-table__identity-secondary">{secondary}</Text>
                ) : null}
            </div>
        );
    };

    const renderRoleTags = (roles: string[] | undefined) => {
        if (!roles || roles.length === 0) {
            return <Text type="secondary" className="admin-users-table__empty-value">{t('users.directory.no_roles')}</Text>;
        }
        return (
            <div className="admin-users-table__roles">
                {roles.map((roleName) => {
                    const role = roleCatalogByName.get(roleName);
                    const label = roleDisplayByName.get(roleName) || roleName;
                    return (
                        <Tag
                            key={roleName}
                            title={roleName}
                            color={getRoleAccessTagColor(role)}
                            className="admin-users-table__role-tag"
                        >
                            {label}
                        </Tag>
                    );
                })}
            </div>
        );
    };

    const openAccessBindingsDrawer = (user: Pick<User, 'id' | 'username' | 'display_name'>) => {
        setSelectedRoleBindingIds([]);
        setSelectedAccessUser({
            id: user.id,
            username: user.username,
            display_name: user.display_name,
        });
    };

    const openBatchAccessModal = () => {
        setSelectedAccessUser(null);
        setSelectedRoleBindingIds([]);
        accessBindings.openAddBindingModal(selectedDirectoryUsers, combinedUserSearch);
    };

    const openSelectedUserAccessModal = () => {
        if (!canManageUserBindings) {
            return;
        }
        const presetUsers = selectedAccessUser
            ? [
                {
                    id: selectedAccessUser.id,
                    username: selectedAccessUser.username,
                    display_name: selectedAccessUser.display_name,
                } as User,
            ]
            : undefined;
        accessBindings.openAddBindingModal(presetUsers);
    };

    const closeAccessBindingsDrawer = () => {
        setSelectedRoleBindingIds([]);
        setSelectedAccessUser(null);
        accessBindings.closeAddBindingModal();
    };

    const handleDeleteSelectedBindings = async () => {
        const { failedIds } = await accessBindings.deleteRoleBindings(visibleSelectedRoleBindingIds);
        setSelectedRoleBindingIds(failedIds);
    };

    const handleResetSelectedUserAccess = async () => {
        const { failedUserIds } = await accessBindings.resetRoleBindingsForUsers(selectedDirectoryUserIds);
        setSelectedDirectoryUserIds(failedUserIds);
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

    const bindingColumns: ColumnsType<GlobalRoleBinding> = [
        {
            title: t('rbac.bindings.role'),
            dataIndex: 'role_name',
            key: 'role_name',
            render: (roleName: string, record: GlobalRoleBinding) => {
                const role = roleCatalogById.get(record.role_id);
                const localizedRoleLabel = role ? localizeRoleLabel(t, role) : record.role_display_name || roleName || record.role_id;
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
            render: (_: unknown, binding: GlobalRoleBinding) => renderBindingScope(binding),
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
            render: (_: unknown, binding: GlobalRoleBinding) => canManageUserBindings ? (
                <Popconfirm
                    title={t('rbac.bindings.delete_confirm')}
                    onConfirm={() => accessBindings.deleteRoleBinding(binding.id)}
                    okText={t('common:button.confirm')}
                    cancelText={t('common:button.cancel')}
                >
                    <Button
                        type="link"
                        danger
                        size="small"
                        data-testid={`user-binding-action-delete-${binding.id}`}
                        loading={accessBindings.deleteBindingPending && accessBindings.deletingBindingId === binding.id}
                    >
                        {t('common:button.delete')}
                    </Button>
                </Popconfirm>
            ) : null,
        },
    ];

    const visibleUserTableColumns = useMemo(
        () => {
            const optionByKey = new Map(userTableColumnOptions.map((option) => [option.key, option] as const));
            return selectedUserTableColumnKeys
                .map((key) => optionByKey.get(key))
                .filter((option): option is UserTableColumnOption => Boolean(option));
        },
        [selectedUserTableColumnKeys, userTableColumnOptions]
    );
    const mergedColumnDraftOptionsById = useMemo(() => {
        const optionByKey = new Map(userTableColumnOptions.map((option) => [option.key, option] as const));
        return new Map(
            mergedColumnDrafts.map((draft) => {
                const claimedByOtherGroups = new Set(
                    mergedColumnDrafts
                        .filter((candidate) => candidate.id !== draft.id)
                        .flatMap((candidate) => candidate.columnKeys)
                );
                const options = columnDraftKeys
                    .map((columnKey) => optionByKey.get(columnKey))
                    .filter((option): option is UserTableColumnOption => Boolean(option))
                    .filter((option) => !claimedByOtherGroups.has(option.key) || draft.columnKeys.includes(option.key));
                return [draft.id, options] as const;
            })
        );
    }, [columnDraftKeys, mergedColumnDrafts, userTableColumnOptions]);

    const hiddenUserTableColumns = useMemo(
        () => {
            const selectedKeySet = new Set(columnsDrawerOpen ? columnDraftKeys : selectedUserTableColumnKeys);
            return userTableColumnOptions.filter((option) => !selectedKeySet.has(option.key));
        },
        [columnDraftKeys, columnsDrawerOpen, selectedUserTableColumnKeys, userTableColumnOptions]
    );

    const orderedUserTableDisplayColumns = useMemo(
        () => buildOrderedUserTableDisplayColumns(visibleUserTableColumns, selectedMergedColumns),
        [selectedMergedColumns, visibleUserTableColumns]
    );
    const usersTableScrollX = useMemo(
        () => estimateUsersTableScrollWidth(orderedUserTableDisplayColumns.length),
        [orderedUserTableDisplayColumns.length]
    );

    const usersColumns: ColumnsType<User> = [
            {
                title: t('users.table.account', { defaultValue: 'Account' }),
                dataIndex: 'username',
                key: USER_TABLE_FIXED_IDENTITY_COLUMN_KEY,
                width: 280,
                fixed: 'left' as const,
                render: (_: unknown, record: User) => renderUserIdentity(record),
            },
            ...orderedUserTableDisplayColumns.map((displayColumn) => {
                if (displayColumn.kind === 'merged') {
                    return {
                        title: displayColumn.label || t('users.directory.merged_column_default_label', {
                            defaultValue: 'Combined details',
                        }),
                        key: `merged-column-${displayColumn.index}`,
                        width: 240,
                        render: (_: unknown, record: User) => {
                            const items = displayColumn.columns
                                .map((column) => ({
                                    label: column.label,
                                    value: (() => {
                                        if (column.kind === 'profile') {
                                            return stringifyUserProfileAttributeValue(
                                                record.profile_attributes?.[column.profileKey ?? ''],
                                            );
                                        }
                                        if (column.key === 'email') {
                                            return record.email || EMPTY_VALUE;
                                        }
                                        if (column.key === 'roles') {
                                            return record.roles && record.roles.length > 0
                                                ? record.roles.map((roleName) => roleDisplayByName.get(roleName) || roleName).join(', ')
                                                : EMPTY_VALUE;
                                        }
                                        if (column.key === 'status') {
                                            return record.enabled ? t('users.status.enabled') : t('users.status.disabled');
                                        }
                                        if (column.key === 'created_at') {
                                            return formatAdminUsersLocalDateTime(record.created_at, displayTimeZone) || EMPTY_VALUE;
                                        }
                                        return EMPTY_VALUE;
                                    })(),
                                }))
                                .filter((item) => item.value !== EMPTY_VALUE);
                            if (items.length === 0) {
                                return <Text type="secondary" className="admin-users-table__empty-value">{EMPTY_VALUE}</Text>;
                            }
                            if (!displayColumn.showLabels) {
                                return (
                                    <div className="admin-users-table__merged-values">
                                        {items.map((item) => (
                                            <span key={item.label} className="admin-users-table__merged-pill">
                                                <Text ellipsis={{ tooltip: item.value }}>{item.value}</Text>
                                            </span>
                                        ))}
                                    </div>
                                );
                            }
                            return (
                                <div className="admin-users-table__merged-values admin-users-table__merged-values--labeled">
                                    {items.map((item) => (
                                        <span key={item.label} className="admin-users-table__merged-pill admin-users-table__merged-pill--labeled">
                                            <Text type="secondary" className="admin-users-table__merged-pill-label">
                                                {item.label}
                                            </Text>
                                            <Text ellipsis={{ tooltip: item.value }}>{item.value}</Text>
                                        </span>
                                    ))}
                                </div>
                            );
                        },
                    };
                }
                const column = displayColumn.column;
                const profileKey = column.kind === 'profile' ? column.profileKey : undefined;
                if (profileKey) {
                    return {
                        title: column.label,
                        key: column.key,
                        width: 170,
                        render: (_: unknown, record: User) => {
                            const value = stringifyUserProfileAttributeValue(record.profile_attributes?.[profileKey]);
                            if (value === EMPTY_VALUE) {
                                return <Text type="secondary" className="admin-users-table__empty-value">{EMPTY_VALUE}</Text>;
                            }
                            return <Text className="admin-users-table__single-line" ellipsis={{ tooltip: value }}>{value}</Text>;
                        },
                    };
                }
                if (column.key === 'email') {
                    return {
                        title: column.label,
                        key: column.key,
                        width: 220,
                        render: (_: unknown, record: User) => {
                            const value = record.email || EMPTY_VALUE;
                            if (value === EMPTY_VALUE) {
                                return <Text type="secondary" className="admin-users-table__empty-value">{EMPTY_VALUE}</Text>;
                            }
                            return <Text className="admin-users-table__single-line" ellipsis={{ tooltip: value }}>{value}</Text>;
                        },
                    };
                }
                if (column.key === 'roles') {
                    return {
                        title: column.label,
                        key: column.key,
                        width: 190,
                        render: (_: unknown, record: User) => renderRoleTags(record.roles),
                    };
                }
                if (column.key === 'status') {
                    return {
                        title: column.label,
                        key: column.key,
                        width: 110,
                        render: (_: unknown, record: User) => (
                            <Tag color={record.enabled ? 'green' : 'default'} className="admin-users-table__status-tag">
                                {record.enabled ? t('users.status.enabled') : t('users.status.disabled')}
                            </Tag>
                        ),
                    };
                }
                return {
                    title: column.label,
                    key: column.key,
                    width: 130,
                    render: (_: unknown, record: User) => {
                        const displayDate = formatAdminUsersLocalDate(record.created_at, displayTimeZone) || EMPTY_VALUE;
                        const fullDateTime = formatAdminUsersLocalDateTime(record.created_at, displayTimeZone);
                        return (
                            <Text
                                className="admin-users-table__single-line admin-users-table__created-at"
                                title={fullDateTime ?? undefined}
                            >
                                {displayDate}
                            </Text>
                        );
                    },
                };
            }),
            {
                title: t('common:table.actions'),
                key: USER_TABLE_FIXED_ACTIONS_COLUMN_KEY,
                width: 148,
                render: (_: unknown, record: User) => (
                    <Space size={0} wrap className="admin-users-table__actions">
                        {canManageUsers ? (
                            <Tooltip title={t('common:button.edit')}>
                                <Button
                                    type="text"
                                    size="small"
                                    icon={<EditOutlined />}
                                    aria-label={t('common:button.edit')}
                                    data-testid={`user-action-edit-${record.id}`}
                                    onClick={() => users.openEditUserModal(record)}
                                />
                            </Tooltip>
                        ) : null}
                        {canReadUserBindings ? (
                            <Tooltip title={t('users.directory.manage_access')}>
                                <Button
                                    type="text"
                                    size="small"
                                    icon={<SafetyCertificateOutlined />}
                                    aria-label={t('users.directory.manage_access')}
                                    data-testid={`user-action-role-bindings-${record.id}`}
                                    onClick={() => openAccessBindingsDrawer(record)}
                                />
                            </Tooltip>
                        ) : null}
                        {canManageUsers ? (
                            <Popconfirm
                                title={t('users.directory.delete_confirm', { username: record.username })}
                                onConfirm={() => users.deleteUser(record.id)}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                            >
                                <Tooltip title={t('common:button.delete')}>
                                    <Button
                                        type="text"
                                        size="small"
                                        danger
                                        icon={<DeleteOutlined />}
                                        aria-label={t('common:button.delete')}
                                        data-testid={`user-action-delete-${record.id}`}
                                        loading={users.deleteUserPending && users.deletingUserId === record.id}
                                    />
                                </Tooltip>
                            </Popconfirm>
                        ) : null}
                    </Space>
                ),
            },
        ];

    const editingUser = useMemo(
        () => (users.users?.items ?? []).find((u) => u.id === users.editingUserId),
        [users.editingUserId, users.users?.items]
    );

    return (
        <div data-testid="admin-users-page" className="admin-users-page">
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
                    title={t('users.summary.roles_title')}
                    value={usersWithExplicitRoles}
                    description={t('users.summary.roles_description')}
                    visual={<RoleCatalogGlyph className="summary-metric-card__art" />}
                    accentColor="#D97706"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('users.summary.profile_title')}
                    value={usersWithDirectoryProfiles}
                    description={t('users.summary.profile_description')}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface className="admin-users-page__table-surface" style={{ marginBottom: 16 }}>
                <Space className="admin-users-page__toolbar" style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                    <Space direction="vertical" size={0} className="admin-users-page__section-heading">
                        <Text strong>{t('users.directory.title')}</Text>
                        <Text type="secondary">{t('users.directory.subtitle')}</Text>
                        <Space wrap size={[8, 4]} className="admin-users-page__workspace-links">
                            <Button size="small" onClick={() => router.push('/admin/rbac')}>
                                {t('users.directory.open_rbac')}
                            </Button>
                            <Button size="small" onClick={() => router.push('/systems')}>
                                {t('users.directory.open_systems')}
                            </Button>
                            <Button size="small" onClick={() => router.push('/admin/rate-limits')}>
                                {t('users.directory.open_rate_limits')}
                            </Button>
                        </Space>
                    </Space>
                    <Space wrap>
                        <Text type="secondary">
                            {t('users.directory.selected_users', {
                                defaultValue: 'Selected {{count}} users',
                                count: selectedDirectoryUserIds.length,
                            })}
                        </Text>
                        <Button
                            onClick={() => setSelectedDirectoryUserIds([])}
                            disabled={selectedDirectoryUserIds.length === 0}
                        >
                            {t('users.directory.batch_manage_access_clear_selection', {
                                defaultValue: 'Clear selection',
                            })}
                        </Button>
                        <Button icon={<ReloadOutlined />} onClick={() => users.refetchUsers()}>
                            {t('common:button.refresh')}
                        </Button>
                        {canManageUserBindings ? (
                            <Button
                                icon={<SafetyCertificateOutlined />}
                                onClick={openBatchAccessModal}
                                data-testid="user-batch-access-button"
                                disabled={selectedDirectoryUserIds.length === 0}
                            >
                                {t('users.directory.batch_manage_access', {
                                    defaultValue: 'Batch access',
                                })}
                            </Button>
                        ) : null}
                        {canManageUserBindings ? (
                            <Popconfirm
                                title={t('users.directory.batch_reset_access_confirm', {
                                    defaultValue: 'Reset explicit access for {{count}} selected users?',
                                    count: selectedDirectoryUserIds.length,
                                })}
                                onConfirm={() => handleResetSelectedUserAccess()}
                                okText={t('common:button.confirm')}
                                cancelText={t('common:button.cancel')}
                                disabled={selectedDirectoryUserIds.length === 0}
                            >
                                <Button
                                    danger
                                    icon={<DeleteOutlined />}
                                    data-testid="user-batch-reset-access-button"
                                    disabled={selectedDirectoryUserIds.length === 0}
                                    loading={accessBindings.deleteBindingPending}
                                >
                                    {t('users.directory.batch_reset_access', {
                                        defaultValue: 'Reset access',
                                    })}
                                </Button>
                            </Popconfirm>
                        ) : null}
                        {canManageUsers ? (
                            <Button type="primary" icon={<PlusOutlined />} data-testid="user-create-button" onClick={users.openCreateUserModal}>
                                {t('users.directory.add')}
                            </Button>
                        ) : null}
                    </Space>
                </Space>
                <Space direction="vertical" size={4} className="admin-users-page__search-stack" style={{ width: '100%', marginTop: 16 }}>
                    <PageSearchToolbar
                        searchValue={quickSearch}
                        searchDraftValue={quickSearchDraft}
                        onSearchDraftChange={setQuickSearchDraft}
                        onSearchChange={applyQuickSearch}
                        searchPlaceholder={t('users.directory.quick_search_placeholder')}
                        searchHelp={t('users.directory.quick_search_help')}
                        searchTestId="users-directory-search"
                        secondaryActions={(
                            <Button
                                icon={<SettingOutlined />}
                                onClick={openColumnsDrawer}
                                data-testid="users-directory-open-columns-drawer"
                            >
                                {t('users.directory.visible_columns_placeholder', {
                                    defaultValue: 'Directory display config',
                                })}
                            </Button>
                        )}
                        advancedSearch={{
                            open: advancedSearchOpen,
                            onToggle: toggleAdvancedSearch,
                            openLabel: t('users.directory.show_advanced_search'),
                            closeLabel: t('users.directory.hide_advanced_search'),
                            toggleTestId: 'users-directory-advanced-search-toggle',
                            content: (
                                <Space direction="vertical" size={8} style={{ width: '100%' }}>
                                    <Text strong>{t('users.directory.advanced_search_title')}</Text>
                                    <Text type="secondary" style={{ fontSize: 13 }}>
                                        {t('users.directory.advanced_search_help')}
                                    </Text>
                                    {advancedSearchDraftConditions.map((condition, index) => {
                                        const valueOptions = userSearchValueOptionsByField.get(condition.field);
                                        return (
                                        <Space
                                            key={`${condition.field}-${index}`}
                                            align="start"
                                            wrap
                                            data-testid={`users-directory-search-condition-row-${index}`}
                                        >
                                            <Select
                                                style={{ minWidth: 220 }}
                                                showSearch
                                                optionFilterProp="label"
                                                placeholder={t('users.directory.advanced_search_field')}
                                                value={condition.field || undefined}
                                                data-testid={`users-directory-search-condition-field-${index}`}
                                                options={userSearchFieldOptions}
                                                onChange={(value) => {
                                                    setAdvancedSearchDraftConditions((current) =>
                                                        current.map((item, itemIndex) =>
                                                            itemIndex === index ? { field: String(value), value: '' } : item
                                                        )
                                                    );
                                                }}
                                            />
                                            {valueOptions?.kind === 'select' ? (
                                                <Select
                                                    allowClear
                                                    showSearch
                                                    optionFilterProp="label"
                                                    style={{ minWidth: 240 }}
                                                    placeholder={t('users.directory.advanced_search_value')}
                                                    value={condition.value || undefined}
                                                    data-testid={`users-directory-search-condition-value-${index}`}
                                                    options={valueOptions.options}
                                                    onChange={(value) => {
                                                        setAdvancedSearchDraftConditions((current) =>
                                                            current.map((item, itemIndex) =>
                                                                itemIndex === index
                                                                    ? { ...item, value: String(value ?? '') }
                                                                    : item
                                                            )
                                                        );
                                                    }}
                                                />
                                            ) : (
                                                <AutoComplete
                                                    style={{ minWidth: 240 }}
                                                    options={valueOptions?.options ?? []}
                                                    placeholder={t('users.directory.advanced_search_value')}
                                                    value={condition.value}
                                                    data-testid={`users-directory-search-condition-value-${index}`}
                                                    filterOption={(inputValue, option) => {
                                                        const search = inputValue.trim().toLowerCase();
                                                        if (!search) {
                                                            return true;
                                                        }
                                                        const label = String(option?.label ?? '').toLowerCase();
                                                        const value = String(option?.value ?? '').toLowerCase();
                                                        return label.includes(search) || value.includes(search);
                                                    }}
                                                    onChange={(value) => {
                                                        setAdvancedSearchDraftConditions((current) =>
                                                            current.map((item, itemIndex) =>
                                                                itemIndex === index ? { ...item, value } : item
                                                            )
                                                        );
                                                    }}
                                                >
                                                    <Input />
                                                </AutoComplete>
                                            )}
                                            <Button
                                                danger
                                                icon={<DeleteOutlined />}
                                                onClick={() => {
                                                    setAdvancedSearchDraftConditions((current) =>
                                                        current.filter((_, itemIndex) => itemIndex !== index)
                                                    );
                                                }}
                                                aria-label={t('users.directory.remove_search_condition')}
                                            />
                                        </Space>
                                        );
                                    })}
                                    <Button
                                        icon={<PlusOutlined />}
                                        onClick={() => {
                                            setAdvancedSearchDraftConditions((current) => [
                                                ...current,
                                                { field: '', value: '' },
                                            ]);
                                        }}
                                        data-testid="users-directory-add-search-condition"
                                    >
                                        {t('users.directory.add_search_condition')}
                                    </Button>
                                    <Button
                                        type="primary"
                                        onClick={applyAdvancedSearch}
                                        data-testid="users-directory-advanced-search-submit"
                                    >
                                        {t('common:button.search')}
                                    </Button>
                                </Space>
                            ),
                        }}
                        hasActiveFilters={Boolean(quickSearch.trim() || advancedSearchConditions.length > 0)}
                        onClear={() => {
                            setQuickSearch('');
                            setQuickSearchDraft('');
                            setAdvancedSearchConditions([]);
                            setAdvancedSearchDraftConditions([]);
                            setAdvancedSearchOpen(false);
                        }}
                        clearLabel={t('users.directory.clear_search')}
                        clearTestId="users-directory-clear-search"
                    />
                </Space>
                <Table<User>
                    className="admin-users-table"
                    style={{ marginTop: 16 }}
                    size="small"
                    rowKey="id"
                    rowSelection={directoryUserRowSelection}
                    columns={usersColumns}
                    dataSource={users.users?.items ?? []}
                    loading={users.usersLoading}
                    scroll={{ x: usersTableScrollX }}
                    rowClassName={() => 'admin-users-table__row'}
                    locale={{
                        emptyText: (
                            <div style={{ padding: 40 }}>
                                <ActionEmptyState
                                    title={t('users.directory.empty')}
                                    description={t('users.directory.empty_description')}
                                    visual={<UserDirectoryGlyph className="action-empty-state__art" />}
                                    actions={canManageUsers ? (
                                        <Button type="primary" icon={<PlusOutlined />} onClick={users.openCreateUserModal}>
                                            {t('users.directory.add')}
                                        </Button>
                                    ) : undefined}
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

            {columnsDrawerOpen ? (
                <Drawer
                    title={t('users.directory.columns_drawer_title', { defaultValue: 'Customize directory display' })}
                    open={columnsDrawerOpen}
                    width={520}
                    onClose={() => setColumnsDrawerOpen(false)}
                    destroyOnClose={true}
                    footer={
                        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                            <Button
                                onClick={() => {
                                    void resetStoredColumnPreference();
                                }}
                                data-testid="users-directory-columns-reset-defaults"
                                disabled={userTablePreference.resetPending}
                            >
                                {t('users.directory.reset_columns', { defaultValue: 'Reset config' })}
                            </Button>
                            <Space>
                                <Button onClick={() => setColumnsDrawerOpen(false)}>
                                    {t('common:button.cancel')}
                                </Button>
                                <Button
                                    type="primary"
                                    onClick={() => {
                                        void saveColumnPreference();
                                    }}
                                    loading={userTablePreference.savePending}
                                    data-testid="users-directory-columns-save"
                                >
                                    {t('common:button.save', { defaultValue: 'Save' })}
                                </Button>
                            </Space>
                        </Space>
                    }
                >
                    <Space direction="vertical" size={16} style={{ width: '100%' }}>
                        <Alert
                            type="info"
                            showIcon
                            message={t('users.directory.columns_drawer_message', {
                                defaultValue: 'Account and Actions stay fixed. Add, hide, and reorder the other columns for your own view.',
                            })}
                        />
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                            <Text strong>{t('users.directory.columns_visible_title', { defaultValue: 'Visible columns' })}</Text>
                            <List
                                bordered
                                size="small"
                                dataSource={columnDraftKeys}
                                locale={{
                                    emptyText: t('users.directory.columns_empty', {
                                        defaultValue: 'No extra columns selected. The table will still show Account and Actions.',
                                    }),
                                }}
                                renderItem={(columnKey) => {
                                    const option = userTableColumnOptions.find((item) => item.key === columnKey);
                                    if (!option) {
                                        return null;
                                    }
                                    const index = columnDraftKeys.indexOf(columnKey);
                                    return (
                                        <List.Item
                                            actions={[
                                                <Button
                                                    key="up"
                                                    type="text"
                                                    size="small"
                                                    icon={<UpOutlined />}
                                                    aria-label={t('users.directory.move_column_up', { defaultValue: 'Move column up' })}
                                                    disabled={index === 0}
                                                    onClick={() => moveDraftColumn(columnKey, 'up')}
                                                />,
                                                <Button
                                                    key="down"
                                                    type="text"
                                                    size="small"
                                                    icon={<DownOutlined />}
                                                    aria-label={t('users.directory.move_column_down', { defaultValue: 'Move column down' })}
                                                    disabled={index === columnDraftKeys.length - 1}
                                                    onClick={() => moveDraftColumn(columnKey, 'down')}
                                                />,
                                                <Button
                                                    key="remove"
                                                    type="text"
                                                    size="small"
                                                    danger
                                                    icon={<DeleteOutlined />}
                                                    aria-label={t('users.directory.hide_column', { defaultValue: 'Hide column' })}
                                                    onClick={() => removeDraftColumn(columnKey)}
                                                />,
                                            ]}
                                        >
                                            <Text>{option.label}</Text>
                                        </List.Item>
                                    );
                                }}
                            />
                        </Space>
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                            <Text strong>{t('users.directory.columns_add_title', { defaultValue: 'Add column' })}</Text>
                            <Select
                                showSearch
                                allowClear
                                placeholder={t('users.directory.columns_add_placeholder', { defaultValue: 'Choose another column to show' })}
                                options={hiddenUserTableColumns.map((option) => ({
                                    value: option.key,
                                    label: option.label,
                                }))}
                                onChange={(value) => {
                                    if (value) {
                                        addDraftColumn(String(value));
                                    }
                                }}
                                disabled={hiddenUserTableColumns.length === 0}
                                data-testid="users-directory-columns-add"
                                optionFilterProp="label"
                            />
                        </Space>
                        <Space direction="vertical" size={8} style={{ width: '100%' }}>
                            <Text strong>{t('users.directory.columns_merge_title', { defaultValue: 'Combined columns' })}</Text>
                            <Text type="secondary" style={{ fontSize: 13 }}>
                                {t('users.directory.columns_merge_help', {
                                    defaultValue: 'Create one or more combined columns from the currently visible columns.',
                                })}
                            </Text>
                            <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                {mergedColumnDrafts.map((draft, index) => (
                                    <Card
                                        key={draft.id}
                                        size="small"
                                        style={{
                                            width: '100%',
                                            background: 'var(--ant-color-fill-quaternary)',
                                            borderColor: 'var(--ant-color-border-secondary)',
                                        }}
                                        data-testid={`users-directory-columns-merge-row-${index}`}
                                    >
                                        <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                            <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                                                <Text strong>
                                                    {t('users.directory.columns_merge_group_title', {
                                                        defaultValue: 'Combined column',
                                                    })} {index + 1}
                                                </Text>
                                                <Button
                                                    type="text"
                                                    size="small"
                                                    danger
                                                    icon={<DeleteOutlined />}
                                                    onClick={() => removeMergedColumnDraft(draft.id)}
                                                    data-testid={`users-directory-columns-merge-remove-${index}`}
                                                >
                                                    {t('users.directory.columns_merge_remove', { defaultValue: 'Remove' })}
                                                </Button>
                                            </Space>
                                            <Input
                                                value={draft.label}
                                                onChange={(event) => updateMergedColumnDraft(draft.id, { label: event.target.value })}
                                                placeholder={t('users.directory.columns_merge_label_placeholder', {
                                                    defaultValue: 'Name this combined column',
                                                })}
                                                data-testid={`users-directory-columns-merge-label-${index}`}
                                            />
                                            <Select
                                                mode="multiple"
                                                value={draft.columnKeys}
                                                style={{ width: '100%' }}
                                                placeholder={t('users.directory.columns_merge_placeholder', {
                                                    defaultValue: 'Select columns to combine',
                                                })}
                                                options={(mergedColumnDraftOptionsById.get(draft.id) ?? []).map((option) => ({
                                                    value: option.key,
                                                    label: option.label,
                                                }))}
                                                onChange={(values) => {
                                                    updateMergedColumnDraft(draft.id, {
                                                        columnKeys: values.map((value) => String(value)),
                                                    });
                                                }}
                                                disabled={(mergedColumnDraftOptionsById.get(draft.id) ?? []).length === 0}
                                                data-testid={`users-directory-columns-merge-select-${index}`}
                                                optionFilterProp="label"
                                            />
                                            {draft.columnKeys.length > 0 ? (
                                                <Space size={[6, 6]} wrap>
                                                    {draft.columnKeys
                                                        .map((key) => (mergedColumnDraftOptionsById.get(draft.id) ?? []).find((option) => option.key === key))
                                                        .filter((option): option is UserTableColumnOption => Boolean(option))
                                                        .map((option) => (
                                                            <Tag key={option.key} color="blue">
                                                                {option.label}
                                                            </Tag>
                                                        ))}
                                                </Space>
                                            ) : null}
                                            <Space
                                                style={{
                                                    width: '100%',
                                                    justifyContent: 'space-between',
                                                    padding: '8px 10px',
                                                    borderRadius: 8,
                                                    background: 'var(--ant-color-bg-container)',
                                                }}
                                                wrap
                                            >
                                                <Space direction="vertical" size={0}>
                                                    <Text strong>
                                                        {t('users.directory.columns_merge_show_labels_title', {
                                                            defaultValue: 'Show field labels inside the column',
                                                        })}
                                                    </Text>
                                                    <Text type="secondary" style={{ fontSize: 13 }}>
                                                        {t('users.directory.columns_merge_show_labels_help', {
                                                            defaultValue: 'Turn this off for a cleaner stacked value view.',
                                                        })}
                                                    </Text>
                                                </Space>
                                                <Switch
                                                    checked={draft.showLabels}
                                                    onChange={(checked) => updateMergedColumnDraft(draft.id, { showLabels: checked })}
                                                    data-testid={`users-directory-columns-merge-show-labels-${index}`}
                                                />
                                            </Space>
                                        </Space>
                                    </Card>
                                ))}
                                <Button
                                    icon={<PlusOutlined />}
                                    onClick={addMergedColumnDraft}
                                    data-testid="users-directory-columns-merge-add"
                                    disabled={columnDraftKeys.length === 0}
                                >
                                    {t('users.directory.columns_merge_add', { defaultValue: 'Add combined column' })}
                                </Button>
                            </Space>
                        </Space>
                        <Button
                            onClick={resetDraftColumns}
                            data-testid="users-directory-columns-restore-defaults"
                        >
                            {t('users.directory.columns_restore_defaults', { defaultValue: 'Restore recommended display config' })}
                        </Button>
                    </Space>
                </Drawer>
            ) : null}

            {selectedAccessUser ? (
                <Drawer
                    title={t('users.directory.manage_access_title', {
                        user: selectedAccessUser.display_name?.trim() || selectedAccessUser.username || selectedAccessUser.id,
                    })}
                    open={Boolean(selectedAccessUser)}
                    width={920}
                    onClose={closeAccessBindingsDrawer}
                    destroyOnClose={true}
                    footer={
                        <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                            <Text type="secondary">
                                {t('users.directory.manage_access_footer', {
                                    defaultValue: 'Use role, scope, and allowed environments together to keep access narrow. Elevated platform-facing roles are highlighted here; Access & Roles remains available for audit.',
                                })}
                            </Text>
                            <Space>
                                <Button onClick={closeAccessBindingsDrawer}>
                                    {t('common:button.close', { defaultValue: 'Close' })}
                                </Button>
                                {canManageUserBindings ? (
                                    <Button
                                        type="primary"
                                        icon={<PlusOutlined />}
                                        data-testid="user-binding-create-button"
                                        onClick={() => accessBindings.openAddBindingModal()}
                                    >
                                        {t('rbac.bindings.add')}
                                    </Button>
                                ) : null}
                            </Space>
                        </Space>
                    }
                >
                    <Space direction="vertical" size={16} style={{ width: '100%' }}>
                        <Alert
                            showIcon
                            type="info"
                            message={t('users.directory.manage_access_help_title')}
                            description={t('users.directory.manage_access_help_description')}
                            className="admin-users-page__access-alert"
                        />
                        <div className="summary-card-grid">
                            <SummaryMetricCard
                                title={t('users.directory.manage_access_summary.standard_title')}
                                value={runtimeBindings.length}
                                description={t('users.directory.manage_access_summary.standard_description')}
                                visual={<AccessControlGlyph className="summary-metric-card__art" />}
                                accentColor="#1D5BFF"
                                surfaceColor="#E6F4FF"
                            />
                            <SummaryMetricCard
                                title={t('users.directory.manage_access_summary.privileged_title')}
                                value={elevatedBindings.length}
                                description={t('users.directory.manage_access_summary.privileged_description')}
                                visual={<SafetyCertificateOutlined />}
                                accentColor="#6D4DE3"
                                surfaceColor="#F5EDFF"
                            />
                        </div>
                        <Space style={{ width: '100%', justifyContent: 'space-between' }} align="start" wrap>
                            <Space direction="vertical" size={0}>
                                <Text strong>{t('users.directory.manage_access_bindings_title')}</Text>
                                <Text type="secondary">{t('users.directory.manage_access_bindings_subtitle')}</Text>
                            </Space>
                            <Space size={12} wrap>
                                <Text type="secondary">
                                    {t('users.directory.manage_access_bindings_selected', {
                                        defaultValue: 'Selected {{count}} bindings',
                                        count: visibleSelectedRoleBindingIds.length,
                                    })}
                                </Text>
                                {canManageUserBindings ? (
                                    <Popconfirm
                                        title={t('users.directory.batch_delete_bindings_confirm', {
                                            defaultValue: 'Remove {{count}} selected bindings?',
                                            count: visibleSelectedRoleBindingIds.length,
                                        })}
                                        onConfirm={() => handleDeleteSelectedBindings()}
                                        okText={t('common:button.delete')}
                                        cancelText={t('common:button.cancel')}
                                        disabled={visibleSelectedRoleBindingIds.length === 0}
                                    >
                                        <Button
                                            danger
                                            icon={<DeleteOutlined />}
                                            disabled={visibleSelectedRoleBindingIds.length === 0}
                                            loading={accessBindings.deleteBindingPending}
                                            data-testid="user-binding-batch-delete-button"
                                        >
                                            {t('users.directory.batch_delete_bindings', {
                                                defaultValue: 'Remove selected',
                                            })}
                                        </Button>
                                    </Popconfirm>
                                ) : null}
                            </Space>
                        </Space>
                        <Table<GlobalRoleBinding>
                            rowKey="id"
                            columns={bindingColumns}
                            dataSource={accessBindings.roleBindings}
                            loading={accessBindings.roleBindingsLoading}
                            rowSelection={canManageUserBindings ? accessBindingRowSelection : undefined}
                            locale={{
                                emptyText: (
                                    <div style={{ padding: 32 }}>
                                        <ActionEmptyState
                                            compact={true}
                                            title={t('users.directory.manage_access_empty')}
                                            description={t('users.directory.manage_access_empty_description')}
                                            visual={<AccessControlGlyph className="action-empty-state__art" />}
                                            actions={canManageUserBindings ? (
                                                <Button type="primary" size="small" icon={<PlusOutlined />} onClick={openSelectedUserAccessModal}>
                                                    {t('rbac.bindings.add')}
                                                </Button>
                                            ) : undefined}
                                        />
                                    </div>
                                ),
                            }}
                            pagination={false}
                        />
                    </Space>
                </Drawer>
            ) : null}

            {accessBindings.addBindingOpen ? (
                <Modal
                    title={t('users.directory.add_binding_title', {
                        user: selectedAccessUser?.display_name?.trim() || selectedAccessUser?.username || selectedAccessUser?.id || t('users.directory.batch_manage_access', {
                            defaultValue: 'Batch access',
                        }),
                    })}
                    open={accessBindings.addBindingOpen}
                    width={960}
                    onOk={() => {
                        void accessBindings.submitAddBinding();
                    }}
                    onCancel={accessBindings.closeAddBindingModal}
                    confirmLoading={accessBindings.createBindingPending}
                    maskClosable={false}
                    keyboard={false}
                    data-testid="user-binding-add-modal"
                >
                    <Form form={accessBindings.bindingForm} layout="vertical" preserve={false}>
                        <Form.Item
                            label={t('rbac.bindings.select_users', {
                                defaultValue: 'Users',
                            })}
                            extra={t('users.directory.batch_manage_access_help', {
                                defaultValue: 'Search by name, email, department, section, or job title, then select multiple users at once.',
                            })}
                        >
                            <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                <UserDirectorySelectionPanel<User>
                                    t={t}
                                    translateDirectoryLabel={t}
                                    users={accessBindings.bindingUserCandidates}
                                    profileFields={accessBindings.bindingUserCandidateProfileFields}
                                    loading={accessBindings.bindingUserCandidatesLoading}
                                    selectedUserIds={selectedBindingUserIds}
                                    onSelectedUserIdsChange={accessBindings.setSelectedBindingUsers}
                                    selectedPreviewUsers={accessBindings.selectedBindingUsers}
                                    selectedPreviewTitle={t('users.directory.batch_manage_access_selection_title', {
                                        defaultValue: 'Users selected for this batch change',
                                    })}
                                    selectedPreviewDescription={t('users.directory.batch_manage_access_add_more_hint', {
                                        defaultValue: 'Search below only if you need to add more users beyond the current selection.',
                                    })}
                                    searchDraft={accessBindings.bindingUserSearchDraft}
                                    appliedSearch={accessBindings.bindingUserSearch}
                                    onSearchDraftChange={accessBindings.setBindingUserSearchDraft}
                                    onSearch={accessBindings.applyBindingUserSearch}
                                    onClearSearch={accessBindings.clearBindingUserSearch}
                                    onClearSelection={accessBindings.clearSelectedBindingUsers}
                                    searchPlaceholder={t('users.directory.select_users_placeholder', {
                                        defaultValue: 'Search and filter users to select in bulk',
                                    })}
                                    searchHelp={t('users.directory.batch_manage_access_help', {
                                        defaultValue: 'Search by name, email, department, section, or job title, then select multiple users at once.',
                                    })}
                                    selectedCountLabel={t('users.directory.batch_manage_access_selected', {
                                        defaultValue: 'Selected {{count}} users',
                                        count: selectedBindingUserIds.length,
                                    })}
                                    clearSelectionLabel={t('users.directory.batch_manage_access_clear_selection', {
                                        defaultValue: 'Clear selection',
                                    })}
                                    noMatchingTitle={t('users.directory.no_matching_users', {
                                        defaultValue: 'No matching users',
                                    })}
                                    noDataTitle={t('users.directory.no_matching_users', {
                                        defaultValue: 'No matching users',
                                    })}
                                    testId="user-binding-user-table"
                                    pagination={{
                                        current: accessBindings.bindingUserPage,
                                        pageSize: accessBindings.bindingUserPerPage,
                                        total: accessBindings.bindingUserCandidatesPagination?.total ?? accessBindings.bindingUserCandidates.length,
                                        onChange: accessBindings.setBindingUserPagination,
                                        showSizeChanger: (accessBindings.bindingUserCandidatesPagination?.total ?? 0) > 50,
                                    }}
                                />
                            </Space>
                        </Form.Item>
                        <Form.Item name="role_id" label={t('rbac.bindings.role')} rules={[{ required: true }]}>
                            <Select
                                options={roleOptions}
                                optionFilterProp="label"
                                showSearch={true}
                                placeholder={t('users.directory.role_placeholder', {
                                    defaultValue: 'Search roles',
                                })}
                            />
                        </Form.Item>
                        {selectedBindingRole ? (
                            <Alert
                                showIcon
                                type={selectedBindingRole && isPrivilegedRole(selectedBindingRole) ? 'warning' : 'info'}
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
                                options={RBAC_SCOPE_VALUES.map((scope) => ({
                                    value: scope,
                                    label: t(`rbac.scope.${scope}`),
                                }))}
                                onChange={(value) => {
                                    accessBindings.bindingForm.setFieldsValue({ scope_type: value, scope_id: undefined });
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
                                    options={(accessBindings.scopeTargetOptionsByType?.[bindingScopeType || 'global'] ?? []).map((option) => ({
                                        value: option.value,
                                        label: option.label,
                                    }))}
                                    allowClear={true}
                                    placeholder={t('rbac.bindings.scope_id_placeholder')}
                                    filterOption={(inputValue, option) => {
                                        const label = String(option?.label ?? '').toLowerCase();
                                        const value = String(option?.value ?? '').toLowerCase();
                                        const search = inputValue.trim().toLowerCase();
                                        return label.includes(search) || value.includes(search);
                                    }}
                                    notFoundContent={accessBindings.scopeTargetLoadingByType?.[bindingScopeType || 'global']
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
                            <Select
                                mode="multiple"
                                options={ENVIRONMENT_VALUES.map((env) => ({
                                    value: env,
                                    label: t(`rbac.env.${env}`),
                                }))}
                            />
                        </Form.Item>
                    </Form>
                </Modal>
            ) : null}

            {users.createUserOpen ? (
                <Modal
                    title={t('users.directory.add_title')}
                    open={users.createUserOpen}
                    onOk={() => {
                        void users.submitCreateUser();
                    }}
                    onCancel={users.closeCreateUserModal}
                    confirmLoading={users.createUserPending}
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
            ) : null}

            {users.editUserOpen ? (
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
            ) : null}
        </div>
    );
}
