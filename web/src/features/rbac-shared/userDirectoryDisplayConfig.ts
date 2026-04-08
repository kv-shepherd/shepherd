'use client';

import type { components } from '@/types/api.gen';

type User = components['schemas']['User'];
type UserProfileField = components['schemas']['UserProfileField'];

const EMPTY_VALUE = '—';
const DEFAULT_VISIBLE_DIRECTORY_PROFILE_COLUMN_COUNT = 2;
const USER_TABLE_CORE_COLUMN_KEYS = ['email', 'roles', 'created_at'] as const;

export const USER_DIRECTORY_DISPLAY_PREFERENCE_KEY = 'admin.users.columns.v4';

export interface UserTableMergedColumnPreference {
    label?: string;
    column_keys?: string[];
    show_labels?: boolean;
}

export interface UserTablePreferenceValue {
    columns?: string[];
    merged_columns?: UserTableMergedColumnPreference[];
    merged_column_keys?: string[];
    merged_column_label?: string;
}

export interface UserTableColumnOption {
    key: string;
    label: string;
    kind: 'core' | 'profile';
    profileKey?: string;
}

export interface NormalizedUserTableMergedColumn {
    label?: string;
    columnKeys: string[];
    showLabels: boolean;
}

export function stringifyUserProfileAttributeValue(value: unknown) {
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

function formatUserDisplayDateTime(value?: string | null) {
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

export function buildDefaultUserTableColumnKeys(fields: UserProfileField[]) {
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

type OrderedUserTableDisplayColumn = ReturnType<typeof buildOrderedUserTableDisplayColumns>[number];

function formatUserDisplayColumnValue(
    record: User,
    column: UserTableColumnOption,
    roleDisplayByName: Map<string, string>,
    t: (key: string, options?: { defaultValue?: string }) => string,
) {
    if (column.kind === 'profile') {
        return stringifyUserProfileAttributeValue(record.profile_attributes?.[column.profileKey ?? '']);
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
        return formatUserDisplayDateTime(record.created_at) || EMPTY_VALUE;
    }
    return EMPTY_VALUE;
}

export function buildUserDisplayFacts(
    record: User,
    displayColumns: OrderedUserTableDisplayColumn[],
    roleDisplayByName: Map<string, string>,
    t: (key: string, options?: { defaultValue?: string }) => string,
) {
    const facts: Array<{ label: string; value: string }> = [];

    for (const displayColumn of displayColumns) {
        if (displayColumn.kind === 'single') {
            const value = formatUserDisplayColumnValue(record, displayColumn.column, roleDisplayByName, t);
            if (value === EMPTY_VALUE) {
                continue;
            }
            facts.push({
                label: displayColumn.column.label,
                value,
            });
            continue;
        }

        const items = displayColumn.columns
            .map((column) => ({
                label: column.label,
                value: formatUserDisplayColumnValue(record, column, roleDisplayByName, t),
            }))
            .filter((item) => item.value !== EMPTY_VALUE);
        if (items.length === 0) {
            continue;
        }
        facts.push({
            label: displayColumn.label || t('users.directory.merged_column_default_label', {
                defaultValue: 'Combined details',
            }),
            value: displayColumn.showLabels
                ? items.map((item) => `${item.label}: ${item.value}`).join(' · ')
                : items.map((item) => item.value).join(' · '),
        });
    }

    return facts;
}
