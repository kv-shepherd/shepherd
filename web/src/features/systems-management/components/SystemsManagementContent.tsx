'use client';

import {
    Button,
    Card,
    Form,
    Input,
    Modal,
    Space,
    Table,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    AppstoreOutlined,
    EditOutlined,
    DeleteOutlined,
    ExclamationCircleOutlined,
    PlusOutlined,
    ReloadOutlined,
    TeamOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { PermissionGuard } from '@/components/auth/PermissionGuard';
import { useSystemsManagementController } from '../hooks/useSystemsManagementController';
import { RFC1035_PATTERN, type System } from '../types';
import { SystemMembersModal } from './SystemMembersModal';

const { Title, Text, Paragraph } = Typography;

export function SystemsManagementContent() {
    const { t } = useTranslation('common');
    const systems = useSystemsManagementController({ t });

    const columns: ColumnsType<System> = [
        {
            title: t('table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string) => (
                <Space>
                    <AppstoreOutlined style={{ color: '#1677ff' }} />
                    <Text strong>{name}</Text>
                </Space>
            ),
        },
        {
            title: t('table.description'),
            dataIndex: 'description',
            key: 'description',
            ellipsis: true,
            render: (desc: string) => <Text type="secondary">{desc || '—'}</Text>,
        },
        {
            title: t('table.created_by'),
            dataIndex: 'created_by',
            key: 'created_by',
            width: 140,
        },
        {
            title: t('table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => (
                <Text type="secondary">{dayjs(date).format('YYYY-MM-DD HH:mm')}</Text>
            ),
        },
        {
            title: t('table.actions'),
            key: 'actions',
            width: 160,
            render: (_, record) => (
                <Space>
                    <PermissionGuard permission="rbac:manage">
                        <Button
                            type="text"
                            data-testid={`system-action-members-${record.id}`}
                            icon={<TeamOutlined />}
                            onClick={() => systems.openMembersModal(record)}
                            title={t('button.manage_members')}
                        />
                    </PermissionGuard>
                    <PermissionGuard permission="system:write">
                        <Button
                            type="text"
                            data-testid={`system-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            loading={systems.updatePending && systems.editingSystem?.id === record.id}
                            onClick={() => systems.openEditModal(record)}
                        />
                    </PermissionGuard>
                    <PermissionGuard permission="system:delete">
                        <Button
                            type="text"
                            data-testid={`system-action-delete-${record.id}`}
                            danger
                            icon={<DeleteOutlined />}
                            loading={systems.deletePending && systems.deletingSystem?.id === record.id}
                            onClick={() => systems.openDeleteModal(record)}
                        />
                    </PermissionGuard>
                </Space>
            ),
        },
    ];

    return (
        <div>
            {systems.messageContextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('nav.systems')}</Title>
                    <Text type="secondary">{t('systems.subtitle')}</Text>
                </div>
                <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => systems.refetch()}>
                        {t('button.refresh')}
                    </Button>
                    <PermissionGuard permission="system:write">
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            data-testid="system-create-button"
                            onClick={systems.openCreateModal}
                        >
                            {t('button.create')}
                        </Button>
                    </PermissionGuard>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<System>
                    columns={columns}
                    dataSource={systems.data?.items ?? []}
                    rowKey="id"
                    loading={systems.isLoading}
                    pagination={{
                        current: systems.page,
                        pageSize: systems.pageSize,
                        total: systems.data?.pagination?.total ?? 0,
                        showTotal: (total) => t('table.total', { total }),
                        onChange: (page, pageSize) => {
                            systems.setPage(page);
                            systems.setPageSize(pageSize);
                        },
                    }}
                    size="middle"
                />
            </Card>

            <Modal
                title={t('systems.modal.create_title')}
                open={systems.createOpen}
                onOk={() => {
                    void systems.submitCreate();
                }}
                onCancel={systems.closeCreateModal}
                confirmLoading={systems.createPending}
                forceRender
            >
                <Form form={systems.form} layout="vertical" name="create-system">
                    <Form.Item
                        name="name"
                        label={t('table.name')}
                        rules={[
                            { required: true, message: t('systems.validation.name_required') },
                            { max: 15, message: t('systems.validation.name_max') },
                            {
                                pattern: RFC1035_PATTERN,
                                message: t('systems.validation.name_format'),
                            },
                        ]}
                    >
                        <Input placeholder={t('systems.name_placeholder')} maxLength={15} />
                    </Form.Item>
                    <Form.Item name="description" label={t('table.description')}>
                        <Input.TextArea rows={3} placeholder={t('systems.description_placeholder')} />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('systems.modal.edit_title')}
                open={systems.editOpen}
                onOk={() => {
                    void systems.submitEdit();
                }}
                onCancel={systems.closeEditModal}
                confirmLoading={systems.updatePending}
                forceRender
            >
                <Form form={systems.editForm} layout="vertical" name="edit-system">
                    <Form.Item name="description" label={t('table.description')}>
                        <Input.TextArea rows={3} placeholder={t('systems.edit_description_placeholder')} />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={(
                    <Space>
                        <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />
                        {t('systems.delete_title')}
                    </Space>
                )}
                open={systems.deleteOpen}
                onOk={systems.submitDelete}
                onCancel={systems.closeDeleteModal}
                confirmLoading={systems.deletePending}
                okButtonProps={{
                    danger: true,
                    disabled: systems.deleteConfirmName !== systems.deletingSystem?.name,
                }}
                okText={t('button.delete')}
            >
                <Paragraph>
                    {t('systems.delete_confirm', { name: systems.deletingSystem?.name })}
                </Paragraph>
                <Paragraph type="secondary">
                    {t('systems.delete_type_name')}
                </Paragraph>
                <Input
                    value={systems.deleteConfirmName}
                    onChange={(e) => systems.setDeleteConfirmName(e.target.value)}
                    placeholder={systems.deletingSystem?.name}
                    status={systems.deleteConfirmName && systems.deleteConfirmName !== systems.deletingSystem?.name ? 'error' : undefined}
                />
            </Modal>

            <SystemMembersModal
                open={systems.membersOpen}
                onCancel={systems.closeMembersModal}
                systemId={systems.membersSystem?.id ?? null}
                systemName={systems.membersSystem?.name}
            />
        </div>
    );
}
