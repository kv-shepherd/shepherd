'use client';

import {
    Alert,
    Button,
    Card,
    Input,
    Space,
    Table,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { TableRowSelection } from 'antd/es/table/interface';
import type { TFunction } from 'i18next';
import { useMemo, type ReactNode } from 'react';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { UserDirectoryGlyph } from '@/components/illustrations/DashboardIllustrations';
import { useUserPreference } from '@/hooks/useUserPreference';
import type { components } from '@/types/api.gen';
import {
    buildDefaultUserTableColumnKeys,
    buildOrderedUserTableDisplayColumns,
    buildUserDisplayFacts,
    buildUserTableColumnOptions,
    normalizeUserTableMergedColumns,
    normalizeUserTablePreferenceColumns,
    type UserTablePreferenceValue,
    USER_DIRECTORY_DISPLAY_PREFERENCE_KEY,
} from './userDirectoryDisplayConfig';

const { Text } = Typography;

type User = components['schemas']['User'];
type UserProfileField = components['schemas']['UserProfileField'];

type DirectorySelectableUser = Pick<User, 'id' | 'username' | 'display_name' | 'email' | 'profile_attributes' | 'roles' | 'enabled' | 'created_at'>;

interface UserDirectorySelectionPanelProps<TUser extends DirectorySelectableUser> {
    t: TFunction;
    translateDirectoryLabel: (key: string, options?: { defaultValue?: string }) => string;
    users: TUser[];
    profileFields: UserProfileField[];
    loading: boolean;
    error?: ReactNode;
    selectedUserIds: string[];
    onSelectedUserIdsChange: (userIds: string[], users: TUser[]) => void;
    selectedPreviewUsers?: TUser[];
    selectedPreviewTitle?: string;
    selectedPreviewDescription?: string;
    searchDraft: string;
    appliedSearch: string;
    onSearchDraftChange: (value: string) => void;
    onSearch: (value?: string) => void;
    onClearSearch: () => void;
    onClearSelection: () => void;
    searchPlaceholder: string;
    searchHelp: string;
    selectedCountLabel: string;
    clearSelectionLabel: string;
    noMatchingTitle: string;
    noDataTitle: string;
    noDataDescription?: string;
    testId?: string;
    pagination: {
        current: number;
        pageSize: number;
        total: number;
        onChange: (page: number, pageSize?: number) => void;
        showSizeChanger?: boolean;
    };
}

export function UserDirectorySelectionPanel<TUser extends DirectorySelectableUser>({
    t,
    translateDirectoryLabel,
    users,
    profileFields,
    loading,
    error,
    selectedUserIds,
    onSelectedUserIdsChange,
    selectedPreviewUsers = [],
    selectedPreviewTitle,
    selectedPreviewDescription,
    searchDraft,
    appliedSearch,
    onSearchDraftChange,
    onSearch,
    onClearSearch,
    onClearSelection,
    searchPlaceholder,
    searchHelp,
    selectedCountLabel,
    clearSelectionLabel,
    noMatchingTitle,
    noDataTitle,
    noDataDescription,
    testId,
    pagination,
}: UserDirectorySelectionPanelProps<TUser>) {
    const userDirectoryDisplayPreference = useUserPreference<UserTablePreferenceValue>(
        USER_DIRECTORY_DISPLAY_PREFERENCE_KEY,
    );
    const roleDisplayByName = useMemo(() => new Map<string, string>(), []);
    const columnOptions = useMemo(
        () => buildUserTableColumnOptions(translateDirectoryLabel, profileFields),
        [profileFields, translateDirectoryLabel],
    );
    const defaultColumnKeys = useMemo(
        () => buildDefaultUserTableColumnKeys(profileFields),
        [profileFields],
    );
    const selectedColumnKeys = useMemo(
        () =>
            normalizeUserTablePreferenceColumns(
                userDirectoryDisplayPreference.value?.columns,
                columnOptions,
                defaultColumnKeys,
            ),
        [columnOptions, defaultColumnKeys, userDirectoryDisplayPreference.value?.columns],
    );
    const selectedMergedColumns = useMemo(
        () =>
            normalizeUserTableMergedColumns(
                userDirectoryDisplayPreference.value?.merged_columns,
                selectedColumnKeys,
                columnOptions,
                userDirectoryDisplayPreference.value?.merged_column_keys,
                userDirectoryDisplayPreference.value?.merged_column_label,
            ),
        [
            columnOptions,
            selectedColumnKeys,
            userDirectoryDisplayPreference.value?.merged_column_keys,
            userDirectoryDisplayPreference.value?.merged_column_label,
            userDirectoryDisplayPreference.value?.merged_columns,
        ],
    );
    const visibleColumns = useMemo(() => {
        const optionByKey = new Map(columnOptions.map((option) => [option.key, option] as const));
        return selectedColumnKeys
            .map((key) => optionByKey.get(key))
            .filter((option): option is NonNullable<typeof option> => Boolean(option));
    }, [columnOptions, selectedColumnKeys]);
    const orderedDisplayColumns = useMemo(
        () => buildOrderedUserTableDisplayColumns(visibleColumns, selectedMergedColumns),
        [selectedMergedColumns, visibleColumns],
    );
    const identityColumn = useMemo<ColumnsType<TUser>>(
        () => [
            {
                title: translateDirectoryLabel('users.table.account', { defaultValue: 'Account' }),
                dataIndex: 'id',
                key: 'identity',
                render: (_value, user) => {
                    const primary = user.display_name?.trim() || user.username || user.id;
                    const secondary = user.email?.trim() || user.username || user.id;
                    const facts = buildUserDisplayFacts(
                        user as User,
                        orderedDisplayColumns,
                        roleDisplayByName,
                        translateDirectoryLabel,
                    ).filter((fact) => fact.label !== translateDirectoryLabel('users.table.email', { defaultValue: 'Email' }));

                    return (
                        <Space direction="vertical" size={2}>
                            <Text strong>{primary}</Text>
                            <Text type="secondary" style={{ fontSize: 13 }}>
                                {secondary}
                            </Text>
                            {facts.map((fact) => (
                                <Text key={fact.label} type="secondary" style={{ fontSize: 13 }}>
                                    {fact.label}: {fact.value}
                                </Text>
                            ))}
                        </Space>
                    );
                },
            },
        ],
        [orderedDisplayColumns, roleDisplayByName, translateDirectoryLabel],
    );
    const rowSelection = useMemo<TableRowSelection<TUser>>(
        () => ({
            selectedRowKeys: selectedUserIds,
            preserveSelectedRowKeys: true,
            onChange: (selectedRowKeys, selectedRows) => {
                onSelectedUserIdsChange(selectedRowKeys.map((value) => String(value)), selectedRows);
            },
        }),
        [onSelectedUserIdsChange, selectedUserIds],
    );
    const emptyText = loading ? (
        t('common:status.loading', { defaultValue: 'Loading…' })
    ) : (
        <ActionEmptyState
            compact={true}
            title={appliedSearch.trim() ? noMatchingTitle : noDataTitle}
            description={appliedSearch.trim() ? searchHelp : noDataDescription}
            visual={<UserDirectoryGlyph className="action-empty-state__art action-empty-state__art--compact" />}
        />
    );

    return (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
            {error ? <Alert showIcon type="warning" message={error} /> : null}

            {selectedPreviewUsers.length > 0 && selectedPreviewTitle ? (
                <Card size="small" title={selectedPreviewTitle}>
                    <Space direction="vertical" size={12} style={{ width: '100%' }}>
                        {selectedPreviewDescription ? (
                            <Text type="secondary">{selectedPreviewDescription}</Text>
                        ) : null}
                        <Table<TUser>
                            rowKey="id"
                            size="small"
                            showHeader={false}
                            pagination={false}
                            columns={identityColumn}
                            dataSource={selectedPreviewUsers}
                        />
                    </Space>
                </Card>
            ) : null}

            <Text type="secondary">{searchHelp}</Text>

            <Input.Search
                allowClear
                enterButton={t('common:button.search')}
                value={searchDraft}
                placeholder={searchPlaceholder}
                onChange={(event) => onSearchDraftChange(event.target.value)}
                onSearch={onSearch}
            />

            <Space style={{ width: '100%', justifyContent: 'space-between' }} wrap>
                <Text type="secondary">{selectedCountLabel}</Text>
                <Space size={8}>
                    <Button
                        type="link"
                        size="small"
                        onClick={onClearSearch}
                        disabled={searchDraft.length === 0 && appliedSearch.length === 0}
                    >
                        {t('common:button.clear')}
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        onClick={onClearSelection}
                        disabled={selectedUserIds.length === 0}
                    >
                        {clearSelectionLabel}
                    </Button>
                </Space>
            </Space>

            <div data-testid={testId}>
                <Table<TUser>
                    rowKey="id"
                    size="small"
                    pagination={{
                        current: pagination.current,
                        pageSize: pagination.pageSize,
                        total: pagination.total,
                        hideOnSinglePage: true,
                        showSizeChanger: pagination.showSizeChanger,
                        onChange: pagination.onChange,
                        showTotal: (total) =>
                            t('common:table.total', { defaultValue: 'Total {{total}} items', total }),
                    }}
                    scroll={{ y: 360 }}
                    columns={identityColumn}
                    dataSource={users}
                    loading={loading}
                    rowSelection={rowSelection}
                    locale={{ emptyText }}
                />
            </div>
        </Space>
    );
}
