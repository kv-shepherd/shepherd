'use client';

import {
    Button,
    Card,
    Form,
    Input,
    Modal,
    Popconfirm,
    Select,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { CloudOutlined, DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { PermissionGuard } from '@/components/auth/PermissionGuard';
import { useServicesManagementController } from '../hooks/useServicesManagementController';
import type { Service } from '../types';

const { Title, Text } = Typography;

export function ServicesManagementContent() {
    const { t } = useTranslation('common');
    const services = useServicesManagementController({ t });

    const columns: ColumnsType<Service> = [
        {
            title: t('table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string) => (
                <Space>
                    <CloudOutlined style={{ color: '#531dab' }} />
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
            title: t('services.instance_index'),
            dataIndex: 'next_instance_index',
            key: 'next_instance_index',
            width: 130,
            render: (idx: number) => <Tag color="blue">{idx ?? 0}</Tag>,
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
            width: 140,
            render: (_, record) => (
                <Space>
                    <PermissionGuard permission="service:create">
                        <Button
                            type="text"
                            size="small"
                            data-testid={`service-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            loading={services.updatePending && services.editingService?.id === record.id}
                            onClick={() => services.openEditModal(record)}
                        />
                    </PermissionGuard>
                    <PermissionGuard permission="service:delete">
                        <Popconfirm
                            title={t('message.confirm_delete')}
                            onConfirm={() => services.submitDelete(record.system_id, record.id)}
                            okText={t('button.confirm')}
                            cancelText={t('button.cancel')}
                        >
                            <Button
                                type="text"
                                size="small"
                                data-testid={`service-action-delete-${record.id}`}
                                danger
                                icon={<DeleteOutlined />}
                                loading={services.deletePending}
                            />
                        </Popconfirm>
                    </PermissionGuard>
                </Space>
            ),
        },
    ];

    return (
        <div>
            {services.messageContextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('nav.services')}</Title>
                    <Text type="secondary">{t('services.subtitle')}</Text>
                </div>
                <Space>
                    <Select
                        data-testid="services-system-selector"
                        style={{ width: 200 }}
                        placeholder={t('services.select_system')}
                        value={services.activeSystemId || undefined}
                        onChange={services.changeSystem}
                        options={services.systemsData?.items?.map((system) => ({
                            label: system.name,
                            value: system.id,
                        }))}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => services.refetch()}>
                        {t('button.refresh')}
                    </Button>
                    <PermissionGuard permission="service:create">
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            data-testid="service-create-button"
                            onClick={services.openCreateModal}
                        >
                            {t('button.create')}
                        </Button>
                    </PermissionGuard>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<Service>
                    columns={columns}
                    dataSource={services.servicesData?.items ?? []}
                    rowKey="id"
                    loading={services.isLoading}
                    pagination={{
                        current: services.page,
                        pageSize: services.pageSize,
                        total: services.servicesData?.pagination?.total ?? 0,
                        showTotal: (total) => t('table.total', { total }),
                        onChange: (page, pageSize) => {
                            services.setPage(page);
                            services.setPageSize(pageSize);
                        },
                    }}
                    size="middle"
                />
            </Card>

            <Modal
                title={t('services.modal.create_title')}
                open={services.createOpen}
                onOk={() => {
                    void services.submitCreate();
                }}
                onCancel={services.closeCreateModal}
                confirmLoading={services.createPending}
                forceRender
            >
                <Form form={services.form} layout="vertical" name="create-service">
                    <Form.Item
                        name="system_id"
                        label={t('services.form.system_label')}
                        rules={[{ required: true, message: t('services.validation.system_required') }]}
                        initialValue={services.activeSystemId}
                    >
                        <Select
                            placeholder={t('services.select_system')}
                            options={services.systemsData?.items?.map((system) => ({
                                label: system.name,
                                value: system.id,
                            }))}
                        />
                    </Form.Item>
                    <Form.Item
                        name="name"
                        label={t('table.name')}
                        rules={[
                            { required: true, message: t('services.validation.name_required') },
                            { max: 15, message: t('services.validation.name_max') },
                        ]}
                    >
                        <Input placeholder={t('services.name_placeholder')} maxLength={15} />
                    </Form.Item>
                    <Form.Item name="description" label={t('table.description')}>
                        <Input.TextArea rows={3} placeholder={t('services.description_placeholder')} />
                    </Form.Item>
                </Form>
            </Modal>

            <Modal
                title={t('services.modal.edit_title')}
                open={services.editOpen}
                onOk={() => {
                    void services.submitEdit();
                }}
                onCancel={services.closeEditModal}
                confirmLoading={services.updatePending}
                forceRender
            >
                <Form form={services.editForm} layout="vertical" name="edit-service">
                    <Form.Item name="description" label={t('table.description')}>
                        <Input.TextArea rows={3} placeholder={t('services.description_placeholder')} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
