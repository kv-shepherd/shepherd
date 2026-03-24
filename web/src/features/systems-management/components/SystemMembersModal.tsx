'use client';

import {
    Button,
    Form,
    Modal,
    Popconfirm,
    Select,
    Space,
    Table,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, PlusOutlined, UserOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { UserDirectoryGlyph } from '@/components/illustrations/DashboardIllustrations';
import { useSystemMembersController } from '../hooks/useSystemMembersController';
import type { SystemMember, SystemMemberRoleUpdateRequest } from '../types';

const { Text } = Typography;

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
    const { t } = useTranslation('common');
    const members = useSystemMembersController({ t, systemId });

    const roleOptions = [
        { label: t('role.owner'), value: 'owner' },
        { label: t('role.admin'), value: 'admin' },
        { label: t('role.member'), value: 'member' },
        { label: t('role.viewer'), value: 'viewer' },
    ];
    const memberCandidateOptions = (members.memberCandidates?.items ?? []).map((user) => ({
        label: user.display_name?.trim() || user.username || user.id,
        value: user.id,
    }));

    const renderUserIdentity = (record: Pick<SystemMember, 'display_name' | 'username' | 'email' | 'user_id'>) => {
        const primary = record.display_name?.trim() || record.username || record.user_id;
        const secondary = record.username && record.username !== primary ? record.username : record.user_id;

        return (
            <Space>
                <UserOutlined />
                <Space direction="vertical" size={0}>
                    <Text strong>{primary}</Text>
                    {secondary ? <Text type="secondary" style={{ fontSize: 12 }}>{secondary}</Text> : null}
                    <Text type="secondary" style={{ fontSize: 12 }}>{record.email || t('members.no_email', 'No email')}</Text>
                </Space>
            </Space>
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
            width: 120,
            render: (_, record) => (
                <Popconfirm
                    title={t('message.confirm_remove_member')}
                    onConfirm={() => members.removeMember(record.user_id)}
                    okText={t('button.confirm')}
                    cancelText={t('button.cancel')}
                >
                    <Button
                        type="link"
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        data-testid={`member-action-remove-${record.user_id}`}
                        loading={members.removeMemberPending}
                    >
                        {t('button.delete')}
                    </Button>
                </Popconfirm>
            ),
        },
    ];

    return (
        <Modal
            title={`${t('button.manage_members')}: ${systemName || ''}`}
            open={open}
            onCancel={onCancel}
            footer={null}
            width={700}
            forceRender={true}
            data-testid="system-members-modal"
        >
            <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end' }}>
                <Button
                    type="primary"
                    icon={<PlusOutlined />}
                    data-testid="member-add-button"
                    onClick={members.openAddMemberModal}
                >
                    {t('button.add_member')}
                </Button>
            </div>

            <Table<SystemMember>
                columns={columns}
                dataSource={members.members}
                rowKey="user_id"
                loading={members.isLoading}
                pagination={false}
                size="small"
                locale={{
                    emptyText: (
                        <ActionEmptyState
                            compact={true}
                            title={t('members.empty', 'No system members yet')}
                            description={t(
                                'members.empty_description',
                                'Add the first member before delegating access from this system to services and virtual machines.',
                            )}
                            visual={<UserDirectoryGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                        />
                    ),
                }}
            />

            <Modal
                title={t('button.add_member')}
                open={members.addMemberOpen}
                onOk={() => {
                    void members.submitAddMember();
                }}
                onCancel={members.closeAddMemberModal}
                confirmLoading={members.addMemberPending}
                forceRender={true}
                data-testid="member-add-modal"
            >
                <Form form={members.addMemberForm} layout="vertical" name="add-system-member" preserve={false}>
                    <Form.Item
                        name="user_id"
                        label={t('members.select_user', 'User')}
                        rules={[{ required: true, message: t('validation.required') }]}
                    >
                        <Select
                            showSearch
                            placeholder={t('members.select_user_placeholder', 'Search for a user who is not yet a member')}
                            data-testid="member-candidate-user-select"
                            filterOption={false}
                            loading={members.memberCandidatesLoading}
                            searchValue={members.memberCandidateSearch}
                            onSearch={members.setMemberCandidateSearch}
                            options={memberCandidateOptions}
                            notFoundContent={
                                members.memberCandidatesLoading
                                    ? t('message.loading')
                                    : members.memberCandidateSearch.trim()
                                        ? t('members.no_search_results', 'No matching users are available to add')
                                        : t('members.no_addable_users', 'All visible users are already members of this system')
                            }
                        />
                    </Form.Item>
                    <Form.Item
                        name="role"
                        label={t('table.role')}
                        rules={[{ required: true, message: t('validation.required') }]}
                        initialValue="member"
                    >
                        <Select options={roleOptions} />
                    </Form.Item>
                </Form>
            </Modal>
        </Modal>
    );
}
