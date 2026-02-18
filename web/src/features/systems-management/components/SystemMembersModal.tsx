'use client';

import {
    Button,
    Form,
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
import { useTranslation } from 'react-i18next';

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

    const columns: ColumnsType<SystemMember> = [
        {
            title: t('table.user'),
            dataIndex: 'user_id', // In a real app, we might want to resolve this to a name
            key: 'user_id',
            render: (userId: string) => (
                <Space>
                    <UserOutlined />
                    <Text>{userId}</Text>
                </Space>
            ),
        },
        {
            title: t('table.role'),
            dataIndex: 'role',
            key: 'role',
            render: (role: string, record) => (
                <Select
                    defaultValue={role}
                    style={{ width: 120 }}
                    onChange={(newRole) => {
                        // Optimistic update or waiting for backend?
                        // Controller handles mutation
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
            width: 80,
            render: (_, record) => (
                <Popconfirm
                    title={t('message.confirm_remove_member')}
                    onConfirm={() => members.removeMember(record.user_id)}
                    okText={t('button.confirm')}
                    cancelText={t('button.cancel')}
                >
                    <Button
                        type="text"
                        danger
                        icon={<DeleteOutlined />}
                        loading={members.removeMemberPending}
                    />
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
            forceRender
        >
            <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end' }}>
                <Button type="primary" icon={<PlusOutlined />} onClick={members.openAddMemberModal}>
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
            />

            <Modal
                title={t('button.add_member')}
                open={members.addMemberOpen}
                onOk={() => {
                    void members.submitAddMember();
                }}
                onCancel={members.closeAddMemberModal}
                confirmLoading={members.addMemberPending}
                forceRender
            >
                <Form form={members.addMemberForm} layout="vertical" name="add-system-member">
                    <Form.Item
                        name="user_id"
                        label={t('table.user_id')}
                        rules={[{ required: true, message: t('validation.required') }]}
                    >
                        <Input placeholder="e.g. user-123" />
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
