'use client';

import {
    Alert,
    Button,
    Card,
    Input,
    Modal,
    Popconfirm,
    Select,
    Space,
    Table,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, PlusOutlined, UserOutlined } from '@ant-design/icons';
import type { Key } from 'react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { UserDirectoryGlyph } from '@/components/illustrations/DashboardIllustrations';
import { UserDirectorySelectionPanel } from '@/features/rbac-shared/UserDirectorySelectionPanel';
import { useSystemMembersController } from '../hooks/useSystemMembersController';
import type { SystemMember, SystemMemberRoleUpdateRequest, UserList } from '../types';
import { translateApiError } from '@/lib/api/errorMessage';

const { Text } = Typography;
type CandidateUser = NonNullable<UserList['items']>[number];

interface SystemMembersModalProps {
    open: boolean;
    onCancel: () => void;
    systemId: string | null;
    systemName?: string;
}

export function SystemMembersModal({
    open,
    onCancel,
    systemId,
    systemName,
}: SystemMembersModalProps) {
    const { t } = useTranslation(['common', 'admin']);
    const members = useSystemMembersController({ t, systemId });
    const [selectedMemberRowIds, setSelectedMemberRowIds] = useState<string[]>([]);
    const [memberSearchDraft, setMemberSearchDraft] = useState('');
    const [memberSearch, setMemberSearch] = useState('');
    const [memberPage, setMemberPage] = useState(1);
    const [memberPerPage, setMemberPerPage] = useState(20);

    const roleOptions = [
        { label: t('role.owner'), value: 'owner' },
        { label: t('role.admin'), value: 'admin' },
        { label: t('role.member'), value: 'member' },
        { label: t('role.viewer'), value: 'viewer' },
    ];
    const filteredMembers = useMemo(() => {
        const keyword = memberSearch.trim().toLowerCase();
        if (!keyword) {
            return members.members;
        }
        return members.members.filter((member) => {
            const profileValues = Object.values(member.profile_attributes ?? {}).flatMap((value) => {
                if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
                    return [String(value)];
                }
                if (Array.isArray(value)) {
                    return value.map((item) => String(item));
                }
                if (value && typeof value === 'object') {
                    return Object.values(value as Record<string, unknown>).map((item) => String(item));
                }
                return [];
            });
            return [
                member.display_name,
                member.username,
                member.email,
                member.user_id,
                member.role,
                ...profileValues,
            ]
                .filter((value): value is string => Boolean(value && value.trim()))
                .some((value) => value.toLowerCase().includes(keyword));
        });
    }, [memberSearch, members.members]);
    const validSelectedMemberRowIds = useMemo(() => {
        const validIds = new Set(members.members.map((member) => member.user_id));
        return selectedMemberRowIds.filter((userId) => validIds.has(userId));
    }, [members.members, selectedMemberRowIds]);
    const visibleMemberPage = useMemo(() => {
        const totalPages = Math.max(1, Math.ceil(filteredMembers.length / memberPerPage));
        return Math.min(memberPage, totalPages);
    }, [filteredMembers.length, memberPage, memberPerPage]);
    const paginatedMembers = useMemo(() => {
        const start = (visibleMemberPage - 1) * memberPerPage;
        return filteredMembers.slice(start, start + memberPerPage);
    }, [filteredMembers, memberPerPage, visibleMemberPage]);
    const memberRowSelection = useMemo(
        () => ({
            preserveSelectedRowKeys: true,
            selectedRowKeys: validSelectedMemberRowIds,
            onChange: (selectedRowKeys: Key[]) => {
                setSelectedMemberRowIds(selectedRowKeys.map((value) => String(value)));
            },
        }),
        [validSelectedMemberRowIds],
    );

    const handleRemoveSelectedMembers = async () => {
        const { failedIds } = await members.removeMembers(validSelectedMemberRowIds);
        setSelectedMemberRowIds(failedIds);
    };
    const applyMemberSearch = (value = memberSearchDraft) => {
        setMemberSearchDraft(value);
        setMemberSearch(value.trim());
        setMemberPage(1);
    };
    const clearMemberSearch = () => {
        setMemberSearchDraft('');
        setMemberSearch('');
        setMemberPage(1);
    };
    const handleClose = () => {
        setSelectedMemberRowIds([]);
        setMemberSearchDraft('');
        setMemberSearch('');
        setMemberPage(1);
        setMemberPerPage(20);
        members.closeAddMemberModal();
        onCancel();
    };

    const renderUserIdentity = (record: SystemMember) => {
        const primary = record.display_name?.trim() || record.username || record.user_id;
        const secondary = record.username && record.username !== primary ? record.username : record.user_id;

        return (
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
                <UserOutlined />
                <div
                    style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 8,
                        minWidth: 0,
                        flexWrap: 'nowrap',
                    }}
                >
                    <Text strong ellipsis={{ tooltip: primary }} style={{ marginBottom: 0 }}>
                        {primary}
                    </Text>
                    {record.email ? (
                        <Text type="secondary" ellipsis={{ tooltip: record.email }} style={{ marginBottom: 0 }}>
                            {record.email}
                        </Text>
                    ) : (
                        <Text type="secondary" style={{ marginBottom: 0 }}>
                            {t('members.no_email', { defaultValue: 'No email' })}
                        </Text>
                    )}
                    {secondary && secondary !== record.email ? (
                        <Text type="secondary" ellipsis={{ tooltip: secondary }} style={{ marginBottom: 0 }}>
                            {secondary}
                        </Text>
                    ) : null}
                </div>
            </div>
        );
    };

    const columns: ColumnsType<SystemMember> = [
        {
            title: t('table.user'),
            dataIndex: 'user_id',
            key: 'user_id',
            render: (_: string, record) => renderUserIdentity(record),
        },
        {
            title: t('table.role'),
            dataIndex: 'role',
            key: 'role',
            render: (role: string, record) => (
                <Select
                    data-testid={`member-action-edit-${record.user_id}`}
                    defaultValue={role}
                    style={{ width: 120 }}
                    onChange={(newRole) => {
                        void members.updateRole(
                            record.user_id,
                            newRole as SystemMemberRoleUpdateRequest['role']
                        );
                    }}
                    options={roleOptions}
                    disabled={members.updateRolePending}
                    variant="borderless"
                />
            ),
        },
        {
            title: t('table.actions'),
            key: 'actions',
            width: 96,
            render: (_, record) => (
                <Popconfirm
                    title={t('message.confirm_remove_member')}
                    onConfirm={() => members.removeMember(record.user_id)}
                    okText={t('common:button.confirm')}
                    cancelText={t('common:button.cancel')}
                >
                    <Button
                        type="link"
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        data-testid={`member-action-remove-${record.user_id}`}
                        loading={members.removeMemberPending && members.removingMemberIds.includes(record.user_id)}
                    >
                        {t('common:button.delete')}
                    </Button>
                </Popconfirm>
            ),
        },
    ];

    return open ? (
        <Modal
            title={`${t('common:button.manage_members')}: ${systemName || ''}`}
            open={true}
            onCancel={handleClose}
            footer={null}
            width={960}
            data-testid="system-members-modal"
        >
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Card
                    size="small"
                    title={t('members.current_members_title', { defaultValue: 'Current members' })}
                >
                    <div
                        style={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                            gap: 12,
                            flexWrap: 'wrap',
                            marginBottom: 12,
                        }}
                    >
                        <Space wrap>
                            <Input.Search
                                value={memberSearchDraft}
                                onChange={(event) => setMemberSearchDraft(event.target.value)}
                                onSearch={applyMemberSearch}
                                enterButton={t('common:button.search')}
                                placeholder={t('members.current_members_search_placeholder', {
                                    defaultValue: 'Search current members by name, email, or role',
                                })}
                                style={{ width: 360, maxWidth: '100%' }}
                                data-testid="member-current-search"
                            />
                            {memberSearch || memberSearchDraft ? (
                                <Button onClick={clearMemberSearch}>
                                    {t('common:button.clear')}
                                </Button>
                            ) : null}
                        </Space>
                        <Space wrap>
                            <Text type="secondary">
                                {t('members.selected_existing_members', {
                                    defaultValue: 'Selected {{count}} members',
                                    count: validSelectedMemberRowIds.length,
                                })}
                            </Text>
                            <Popconfirm
                                title={t('members.batch_remove_confirm', {
                                    defaultValue: 'Remove {{count}} selected members?',
                                    count: validSelectedMemberRowIds.length,
                                })}
                                onConfirm={() => handleRemoveSelectedMembers()}
                                okText={t('common:button.delete')}
                                cancelText={t('common:button.cancel')}
                                disabled={validSelectedMemberRowIds.length === 0}
                            >
                                <Button
                                    danger
                                    icon={<DeleteOutlined />}
                                    data-testid="member-batch-remove-button"
                                    disabled={validSelectedMemberRowIds.length === 0}
                                    loading={members.removeMemberPending}
                                >
                                    {t('members.batch_remove', {
                                        defaultValue: 'Remove selected',
                                    })}
                                </Button>
                            </Popconfirm>
                            <Button
                                type="primary"
                                icon={<PlusOutlined />}
                                data-testid="member-add-button"
                                onClick={members.openAddMemberModal}
                            >
                                {t('common:button.add_member')}
                            </Button>
                        </Space>
                    </div>
                    {members.membersError ? (
                        <Alert
                            showIcon
                            type="warning"
                            message={translateApiError(t, members.membersError, 'message.error')}
                            style={{ marginBottom: 12 }}
                        />
                    ) : null}
                    <Table<SystemMember>
                        columns={columns}
                        dataSource={paginatedMembers}
                        rowKey="user_id"
                        loading={members.isLoading}
                        rowSelection={memberRowSelection}
                        pagination={{
                            current: visibleMemberPage,
                            pageSize: memberPerPage,
                            total: filteredMembers.length,
                            hideOnSinglePage: true,
                            showSizeChanger: filteredMembers.length > 50,
                            showTotal: (total) => t('table.total', { defaultValue: 'Total {{total}} items', total }),
                            onChange: (page, pageSize) => {
                                setMemberPage(page);
                                setMemberPerPage(pageSize);
                            },
                        }}
                        size="small"
                        locale={{
                            emptyText: (
                                <ActionEmptyState
                                    compact={true}
                                    title={
                                        members.membersError
                                            ? t('members.load_failed', { defaultValue: 'Unable to load system members' })
                                            : t('members.empty', { defaultValue: 'No system members yet' })
                                    }
                                    description={
                                        members.membersError
                                            ? translateApiError(t, members.membersError, 'message.error')
                                            : t(
                                                'members.empty_description',
                                                'Add the first member before delegating access from this system to services and virtual machines.',
                                            )
                                    }
                                    visual={<UserDirectoryGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                    actions={
                                        members.membersError ? (
                                            <Button onClick={() => members.refetch()}>
                                                {t('common:button.retry', { defaultValue: 'Retry' })}
                                            </Button>
                                        ) : (
                                            <Button type="primary" icon={<PlusOutlined />} onClick={members.openAddMemberModal}>
                                                {t('common:button.add_member')}
                                            </Button>
                                        )
                                    }
                                />
                            ),
                        }}
                    />
                </Card>

                {members.addMemberOpen ? (
                    <Modal
                        title={t('members.add_workspace_title', { defaultValue: 'Add members in bulk' })}
                        open={true}
                        width={1120}
                        onCancel={members.closeAddMemberModal}
                        onOk={() => {
                            void members.submitAddMember();
                        }}
                        confirmLoading={members.addMemberPending}
                        okText={t('common:button.add_member')}
                        cancelText={t('common:button.cancel')}
                        data-testid="member-add-modal"
                    >
                        <Space direction="vertical" size={16} style={{ width: '100%' }}>
                            <Card size="small" className="system-members-modal__selection-summary">
                                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                                    <Text strong>
                                        {t('members.add_workspace_heading', {
                                            defaultValue: 'Select users to add to this system',
                                        })}
                                    </Text>
                                    <Text type="secondary">
                                        {t(
                                            'members.add_workspace_description',
                                            'Use the same directory search, display, and paging model as user management, then apply one system role to the selected users.',
                                        )}
                                    </Text>
                                    <Space wrap align="center">
                                        <Text type="secondary">{t('table.role')}</Text>
                                        <Select
                                            value={members.addMemberRole}
                                            onChange={(value) =>
                                                members.setAddMemberRole(value as 'admin' | 'member' | 'viewer' | 'owner')
                                            }
                                            options={roleOptions}
                                            style={{ width: 220 }}
                                        />
                                    </Space>
                                </Space>
                            </Card>
                            <UserDirectorySelectionPanel<CandidateUser>
                                t={t}
                                translateDirectoryLabel={(key, options) =>
                                    key.startsWith('users.') ? t(`admin:${key}`, options) : t(key, options)
                                }
                                users={members.memberCandidates?.items ?? []}
                                profileFields={members.memberCandidates?.profile_fields ?? []}
                                loading={members.memberCandidatesLoading}
                                error={
                                    members.memberCandidatesError
                                        ? translateApiError(t, members.memberCandidatesError, 'message.error')
                                        : undefined
                                }
                                selectedUserIds={members.selectedCandidateUserIds}
                                onSelectedUserIdsChange={members.setSelectedCandidateUsers}
                                selectedPreviewUsers={members.selectedCandidateUsers}
                                selectedPreviewTitle={t('members.selected_users', {
                                    defaultValue: 'Selected {{count}} users',
                                    count: members.selectedCandidateUsers.length,
                                })}
                                selectedPreviewDescription={t(
                                    'members.add_workspace_selection_hint',
                                    'Selected users stay pinned here while you continue paging or searching for more people to add.',
                                )}
                                searchDraft={members.memberCandidateSearchDraft}
                                appliedSearch={members.memberCandidateSearch}
                                onSearchDraftChange={members.setMemberCandidateSearchDraft}
                                onSearch={members.applyMemberCandidateSearch}
                                onClearSearch={members.clearMemberCandidateSearch}
                                onClearSelection={() => members.setSelectedCandidateUsers([], [])}
                                searchPlaceholder={t('members.select_user_placeholder', 'Search and filter users to select in bulk')}
                                searchHelp={t('members.select_user_help', 'Search by name, email, department, section, or job title, then select multiple users at once.')}
                                selectedCountLabel={t('members.selected_users', {
                                    defaultValue: 'Selected {{count}} users',
                                    count: members.selectedCandidateUserIds.length,
                                })}
                                clearSelectionLabel={t('members.clear_selection', {
                                    defaultValue: 'Clear selection',
                                })}
                                noMatchingTitle={t('members.no_search_results', 'No matching users are available to add')}
                                noDataTitle={t('members.no_addable_users', 'All visible users are already members of this system')}
                                testId="member-candidate-user-table"
                                pagination={{
                                    current: members.memberCandidatePage,
                                    pageSize: members.memberCandidatePerPage,
                                    total: members.memberCandidates?.pagination?.total ?? (members.memberCandidates?.items ?? []).length,
                                    onChange: members.setMemberCandidatePagination,
                                    showSizeChanger: (members.memberCandidates?.pagination?.total ?? 0) > 50,
                                }}
                            />
                        </Space>
                    </Modal>
                ) : null}
            </Space>
        </Modal>
    ) : null;
}
