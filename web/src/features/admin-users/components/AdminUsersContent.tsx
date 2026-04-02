'use client';

import {
    Alert,
    AutoComplete,
    Button,
    Card,
    DatePicker,
    Drawer,
    Form,
    Input,
    InputNumber,
    List,
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
import {
    DeleteOutlined,
    DownOutlined,
    EditOutlined,
    PlusOutlined,
    ReloadOutlined,
    SettingOutlined,
    TeamOutlined,
    UpOutlined,
} from '@ant-design/icons';
import type { Dayjs } from 'dayjs';
import { useEffect, useMemo, useState } from 'react';
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
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import { useUserPreference } from '@/hooks/useUserPreference';
import { useAdminUsersController } from '../hooks/useAdminUsersController';
import {
    MEMBER_ROLE_VALUES,
    type GlobalRoleBinding,
    type RateLimitUserStatus,
    type SystemMember,
    type SystemMemberRoleUpdateRequest,
    type User,
    type UserProfileField,
} from '../types';

const { Text } = Typography;
const EMPTY_VALUE = '—';
const USER_ROLE_BINDING_SCOPE_VALUES = ['global', 'system', 'service', 'vm'] as const;
const USER_ROLE_BINDING_ENV_VALUES = ['test', 'prod'] as const;
const USER_DIRECTORY_COLUMNS_PREFERENCE_KEY = 'admin.users.columns.v1';
const DEFAULT_VISIBLE_DIRECTORY_PROFILE_COLUMN_COUNT = 2;
const USER_TABLE_FIXED_IDENTITY_COLUMN_KEY = 'identity';
const USER_TABLE_FIXED_ACTIONS_COLUMN_KEY = 'actions';
const USER_TABLE_CORE_COLUMN_KEYS = ['email', 'roles', 'status', 'created_at'] as const;

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

interface UserTableMergedColumnPreference {
    label?: string;
    column_keys?: string[];
    show_labels?: boolean;
}

interface UserTablePreferenceValue {
    columns?: string[];
    merged_columns?: UserTableMergedColumnPreference[];
    merged_column_keys?: string[];
    merged_column_label?: string;
}

interface UserTableColumnOption {
    key: string;
    label: string;
    kind: 'core' | 'profile';
    profileKey?: string;
}

interface UserTableMergedColumnDraft {
    id: string;
    label: string;
    columnKeys: string[];
    showLabels: boolean;
}

interface NormalizedUserTableMergedColumn {
    label?: string;
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

function stringifyUserProfileAttributeValue(value: unknown) {
    if (typeof value === 'string') {
        return value || EMPTY_VALUE;
    }
    if (typeof value === 'number' || typeof value === 'boolean') {
        return String(value);
    }
    return EMPTY_VALUE;
}

function localizeUserProfileFieldLabel(
    t: (key: string, options?: { defaultValue?: string }) => string,
    fieldKey: string,
    fallback: string,
) {
    return t(`users.profile_fields.${fieldKey}`, {
        defaultValue: fallback,
    });
}

function formatAdminUsersLocalDateTime(value?: string | null) {
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
    });
    const parts = formatter.formatToParts(parsed);
    const pick = (type: Intl.DateTimeFormatPartTypes) =>
        parts.find((part) => part.type === type)?.value ?? '';
    return `${pick('year')}-${pick('month')}-${pick('day')} ${pick('hour')}:${pick('minute')}`;
}

export function buildUserTableColumnOptions(
    t: (key: string, options?: { defaultValue?: string }) => string,
    fields: UserProfileField[],
): UserTableColumnOption[] {
    return [
        {
            key: 'email',
            label: t('users.table.email', { defaultValue: 'Email' }),
            kind: 'core',
        },
        {
            key: 'roles',
            label: t('users.table.roles', { defaultValue: 'Roles' }),
            kind: 'core',
        },
        {
            key: 'status',
            label: t('common:table.status', { defaultValue: 'Status' }),
            kind: 'core',
        },
        {
            key: 'created_at',
            label: t('common:table.created_at', { defaultValue: 'Created' }),
            kind: 'core',
        },
        ...fields.map((field) => ({
            key: `profile:${field.key}`,
            label: localizeUserProfileFieldLabel(t, field.key, field.label),
            kind: 'profile' as const,
            profileKey: field.key,
        })),
    ];
}

function buildDefaultUserTableColumnKeys(fields: UserProfileField[]) {
    return [
        ...fields
            .slice(0, DEFAULT_VISIBLE_DIRECTORY_PROFILE_COLUMN_COUNT)
            .map((field) => `profile:${field.key}`),
        ...USER_TABLE_CORE_COLUMN_KEYS,
    ];
}

export function normalizeUserTablePreferenceColumns(
    storedColumns: string[] | undefined,
    availableOptions: UserTableColumnOption[],
    defaultColumns: string[],
) {
    const availableKeySet = new Set(availableOptions.map((option) => option.key));
    const normalized = (storedColumns ?? []).filter((key) => availableKeySet.has(key));
    if (normalized.length === 0) {
        return defaultColumns;
    }
    return normalized;
}

export function normalizeUserTableMergedColumns(
    storedGroups: UserTableMergedColumnPreference[] | undefined,
    selectedColumns: string[],
    availableOptions: UserTableColumnOption[],
    legacyKeys?: string[],
    legacyLabel?: string,
): NormalizedUserTableMergedColumn[] {
    const selectedKeySet = new Set(selectedColumns);
    const availableColumnKeySet = new Set(availableOptions.map((option) => option.key));
    const sourceGroups =
        storedGroups && storedGroups.length > 0
            ? storedGroups
            : legacyKeys && legacyKeys.length > 0
                ? [{ column_keys: legacyKeys, label: legacyLabel }]
                : [];
    const claimedColumnKeys = new Set<string>();
    const normalizedGroups: NormalizedUserTableMergedColumn[] = [];

    for (const group of sourceGroups) {
        const normalizedKeys: string[] = [];
        for (const key of group.column_keys ?? []) {
            if (!selectedKeySet.has(key) || !availableColumnKeySet.has(key) || claimedColumnKeys.has(key)) {
                continue;
            }
            claimedColumnKeys.add(key);
            normalizedKeys.push(key);
        }
        if (normalizedKeys.length === 0) {
            continue;
        }
        normalizedGroups.push({
            label: group.label?.trim() || undefined,
            columnKeys: normalizedKeys,
            showLabels: group.show_labels !== false,
        });
    }

    return normalizedGroups;
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
    return 320 + visibleConfigurableColumnCount * 200 + 220;
}

export function buildOrderedUserTableDisplayColumns(
    visibleUserTableColumns: UserTableColumnOption[],
    selectedMergedColumns: NormalizedUserTableMergedColumn[],
) {
    const optionByKey = new Map(visibleUserTableColumns.map((option) => [option.key, option] as const));
    const mergedGroupIndexByKey = new Map<string, number>();
    selectedMergedColumns.forEach((group, index) => {
        group.columnKeys.forEach((key) => {
            mergedGroupIndexByKey.set(key, index);
        });
    });
    const insertedGroupIndexes = new Set<number>();
    const items: Array<
        | { kind: 'single'; column: UserTableColumnOption }
        | { kind: 'merged'; index: number; label?: string; columns: UserTableColumnOption[]; showLabels: boolean }
    > = [];

    for (const column of visibleUserTableColumns) {
        const mergedGroupIndex = mergedGroupIndexByKey.get(column.key);
        if (mergedGroupIndex === undefined) {
            items.push({ kind: 'single', column });
            continue;
        }
        if (insertedGroupIndexes.has(mergedGroupIndex)) {
            continue;
        }
        const group = selectedMergedColumns[mergedGroupIndex];
        const groupColumns = group.columnKeys
            .map((key) => optionByKey.get(key))
            .filter((option): option is UserTableColumnOption => Boolean(option));
        if (groupColumns.length === 0) {
            continue;
        }
        items.push({
            kind: 'merged',
            index: mergedGroupIndex,
            label: group.label,
            columns: groupColumns,
            showLabels: group.showLabels,
        });
        insertedGroupIndexes.add(mergedGroupIndex);
    }

    return items;
}

export function AdminUsersContent() {
    const { t } = useTranslation(['admin', 'common']);
    const users = useAdminUsersController({ t });
    const { setPage, setSearch } = users;
    const [quickSearch, setQuickSearch] = useState('');
    const [quickSearchDraft, setQuickSearchDraft] = useState('');
    const [advancedSearchOpen, setAdvancedSearchOpen] = useState(false);
    const [advancedSearchConditions, setAdvancedSearchConditions] = useState<AdvancedUserSearchCondition[]>([]);
    const [advancedSearchDraftConditions, setAdvancedSearchDraftConditions] = useState<AdvancedUserSearchCondition[]>([]);
    const [columnsDrawerOpen, setColumnsDrawerOpen] = useState(false);
    const [columnDraftKeys, setColumnDraftKeys] = useState<string[]>([]);
    const [mergedColumnDrafts, setMergedColumnDrafts] = useState<UserTableMergedColumnDraft[]>([]);
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
    const userItems = useMemo(
        () => users.users?.items ?? [],
        [users.users?.items]
    );
    const memberItems = useMemo(
        () => users.members?.items ?? [],
        [users.members?.items]
    );
    const rateLimitItems = useMemo(
        () => users.rateLimitStatus?.items ?? [],
        [users.rateLimitStatus?.items]
    );
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
    const userTablePreference = useUserPreference<UserTablePreferenceValue>(USER_DIRECTORY_COLUMNS_PREFERENCE_KEY);
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
        record: Pick<User, 'username' | 'display_name' | 'email' | 'id'>
        | Pick<SystemMember, 'username' | 'display_name' | 'email' | 'user_id'>
        | Pick<RateLimitUserStatus, 'username' | 'display_name' | 'email' | 'user_id'>
    ) => {
        const username = 'username' in record ? record.username : undefined;
        const displayName = 'display_name' in record ? record.display_name : undefined;
        const identityId = 'id' in record ? record.id : record.user_id;
        const primary = displayName?.trim() || username || identityId;
        const secondary = username && username !== primary ? username : identityId;

        return (
            <Space direction="vertical" size={0}>
                <Text strong>{primary}</Text>
                {secondary ? <Text type="secondary" style={{ fontSize: 12 }}>{secondary}</Text> : null}
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
                width: 320,
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
                        width: 280,
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
                                            return formatAdminUsersLocalDateTime(record.created_at) || EMPTY_VALUE;
                                        }
                                        return EMPTY_VALUE;
                                    })(),
                                }))
                                .filter((item) => item.value !== EMPTY_VALUE);
                            if (items.length === 0) {
                                return <Text type="secondary">{EMPTY_VALUE}</Text>;
                            }
                            if (!displayColumn.showLabels) {
                                return (
                                    <Space direction="vertical" size={6} style={{ width: '100%' }}>
                                        {items.map((item) => (
                                            <div
                                                key={item.label}
                                                style={{
                                                    padding: '8px 10px',
                                                    borderRadius: 10,
                                                    background: 'var(--ant-color-fill-secondary)',
                                                }}
                                            >
                                                <Text ellipsis={{ tooltip: item.value }}>{item.value}</Text>
                                            </div>
                                        ))}
                                    </Space>
                                );
                            }
                            return (
                                <Space direction="vertical" size={6} style={{ width: '100%' }}>
                                    {items.map((item) => (
                                        <div
                                            key={item.label}
                                            style={{
                                                padding: '8px 10px',
                                                borderRadius: 10,
                                                background: 'var(--ant-color-fill-tertiary)',
                                            }}
                                        >
                                            <Text
                                                type="secondary"
                                                style={{
                                                    display: 'block',
                                                    fontSize: 11,
                                                    lineHeight: 1.2,
                                                    marginBottom: 4,
                                                    textTransform: 'uppercase',
                                                    letterSpacing: '0.04em',
                                                }}
                                            >
                                                {item.label}
                                            </Text>
                                            <Text ellipsis={{ tooltip: item.value }}>{item.value}</Text>
                                        </div>
                                    ))}
                                </Space>
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
                        width: 200,
                        render: (_: unknown, record: User) => {
                            const value = stringifyUserProfileAttributeValue(record.profile_attributes?.[profileKey]);
                            if (value === EMPTY_VALUE) {
                                return <Text type="secondary">{EMPTY_VALUE}</Text>;
                            }
                            return <Text ellipsis={{ tooltip: value }}>{value}</Text>;
                        },
                    };
                }
                if (column.key === 'email') {
                    return {
                        title: column.label,
                        key: column.key,
                        width: 260,
                        render: (_: unknown, record: User) => {
                            const value = record.email || EMPTY_VALUE;
                            if (value === EMPTY_VALUE) {
                                return <Text type="secondary">{EMPTY_VALUE}</Text>;
                            }
                            return <Text ellipsis={{ tooltip: value }}>{value}</Text>;
                        },
                    };
                }
                if (column.key === 'roles') {
                    return {
                        title: column.label,
                        key: column.key,
                        width: 220,
                        render: (_: unknown, record: User) => renderRoleTags(record.roles),
                    };
                }
                if (column.key === 'status') {
                    return {
                        title: column.label,
                        key: column.key,
                        width: 140,
                        render: (_: unknown, record: User) => (
                            <Tag color={record.enabled ? 'green' : 'default'}>
                                {record.enabled ? t('users.status.enabled') : t('users.status.disabled')}
                            </Tag>
                        ),
                    };
                }
                return {
                    title: column.label,
                    key: column.key,
                    width: 180,
                        render: (_: unknown, record: User) => <LocalDateTimeText value={record.created_at} />,
                };
            }),
            {
                title: t('common:table.actions'),
                key: USER_TABLE_FIXED_ACTIONS_COLUMN_KEY,
                width: 220,
                fixed: 'right' as const,
                render: (_: unknown, record: User) => (
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
                <Space direction="vertical" size={4} style={{ width: '100%', marginTop: 16 }}>
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
                                    defaultValue: 'Displayed columns',
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
                                    <Text type="secondary" style={{ fontSize: 12 }}>
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
                    style={{ marginTop: 16 }}
                    rowKey="id"
                    columns={usersColumns}
                    dataSource={users.users?.items ?? []}
                    loading={users.usersLoading}
                    scroll={{ x: usersTableScrollX }}
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

            {columnsDrawerOpen ? (
                <Drawer
                    title={t('users.directory.columns_drawer_title', { defaultValue: 'Customize displayed columns' })}
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
                                {t('users.directory.reset_columns', { defaultValue: 'Reset columns' })}
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
                            <Text type="secondary" style={{ fontSize: 12 }}>
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
                                                    <Text type="secondary" style={{ fontSize: 12 }}>
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
                            {t('users.directory.columns_restore_defaults', { defaultValue: 'Restore recommended defaults' })}
                        </Button>
                    </Space>
                </Drawer>
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

            {users.addOpen ? (
                <Modal
                    title={t('users.members.add_title')}
                    open={users.addOpen}
                    onOk={() => {
                        void users.submitAddMember();
                    }}
                    onCancel={users.closeAddModal}
                    confirmLoading={users.addPending}
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
            ) : null}

            {exemptionOpen ? (
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
            ) : null}

            {overrideOpen ? (
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
            ) : null}
            {/* ── User Role Bindings Drawer ─────────────────────────────── */}
            {users.roleBindingsUserId ? (
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
            ) : null}

            {/* ── Create Role Binding Modal ────────────────────────────── */}
            {users.roleBindingCreateOpen ? (
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
            ) : null}
        </div>
    );
}
