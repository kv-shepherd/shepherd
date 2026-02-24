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
    Upload,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import React, { useState } from 'react';
import { CloudOutlined, DeleteOutlined, EditOutlined, EyeOutlined, PlusOutlined, ReloadOutlined, UploadOutlined, FileTextOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSanitize from 'rehype-sanitize';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { PermissionGuard } from '@/components/auth/PermissionGuard';
import { useServicesManagementController } from '../hooks/useServicesManagementController';
import type { Service } from '../types';

const { Title, Text } = Typography;

export function ServicesManagementContent() {
    const { t } = useTranslation('common');
    const services = useServicesManagementController({ t });

    const [createPreviewMode, setCreatePreviewMode] = useState(false);
    const [editPreviewMode, setEditPreviewMode] = useState(false);
    const [detailOpen, setDetailOpen] = useState(false);
    const [detailService, setDetailService] = useState<Service | null>(null);

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
            width: 160,
            render: (_, record) => (
                <Space>
                    <Button
                        type="text"
                        size="small"
                        data-testid={`service-action-detail-${record.id}`}
                        icon={<EyeOutlined />}
                        onClick={() => {
                            setDetailService(record);
                            setDetailOpen(true);
                        }}
                    />
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
                destroyOnHidden={true}
                data-testid="service-create-modal"
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
                    <Form.Item
                        label={t('table.description')}
                        extra={
                            <Space size="small" style={{ marginTop: 8 }}>
                                <Button
                                    type="link"
                                    size="small"
                                    icon={createPreviewMode ? <EditOutlined /> : <FileTextOutlined />}
                                    onClick={() => setCreatePreviewMode(!createPreviewMode)}
                                >
                                    {createPreviewMode ? '[Edit]' : '[Preview]'}
                                </Button>
                                <Upload
                                    accept=".md"
                                    showUploadList={false}
                                    beforeUpload={(file) => {
                                        const reader = new FileReader();
                                        reader.onload = (e) => {
                                            const text = e.target?.result as string;
                                            services.form.setFieldValue('description', text);
                                        };
                                        reader.readAsText(file);
                                        return false;
                                    }}
                                >
                                    <Button type="link" size="small" icon={<UploadOutlined />}>
                                        [Upload .md file]
                                    </Button>
                                </Upload>
                            </Space>
                        }
                    >
                        <Form.Item noStyle shouldUpdate>
                            {(form) => (
                                <div className="markdown-preview" style={{ display: createPreviewMode ? 'block' : 'none', padding: '4px 11px', border: '1px solid #d9d9d9', borderRadius: 6, minHeight: 76, maxHeight: 152, overflowY: 'auto' }}>
                                    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                        {form.getFieldValue('description') || '*No content provided*'}
                                    </ReactMarkdown>
                                </div>
                            )}
                        </Form.Item>
                        <Form.Item name="description" noStyle hidden={createPreviewMode}>
                            <Input.TextArea rows={3} placeholder={t('services.description_placeholder')} />
                        </Form.Item>
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
                destroyOnHidden={true}
                data-testid="service-edit-modal"
            >
                <Form form={services.editForm} layout="vertical" name="edit-service">
                    <Form.Item
                        label={t('table.description')}
                        extra={
                            <Space size="small" style={{ marginTop: 8 }}>
                                <Button
                                    type="link"
                                    size="small"
                                    icon={editPreviewMode ? <EditOutlined /> : <FileTextOutlined />}
                                    onClick={() => setEditPreviewMode(!editPreviewMode)}
                                >
                                    {editPreviewMode ? '[Edit]' : '[Preview]'}
                                </Button>
                                <Upload
                                    accept=".md"
                                    showUploadList={false}
                                    beforeUpload={(file) => {
                                        const reader = new FileReader();
                                        reader.onload = (e) => {
                                            const text = e.target?.result as string;
                                            services.editForm.setFieldValue('description', text);
                                        };
                                        reader.readAsText(file);
                                        return false;
                                    }}
                                >
                                    <Button type="link" size="small" icon={<UploadOutlined />}>
                                        [Upload .md file]
                                    </Button>
                                </Upload>
                            </Space>
                        }
                    >
                        <Form.Item noStyle shouldUpdate>
                            {(form) => (
                                <div className="markdown-preview" style={{ display: editPreviewMode ? 'block' : 'none', padding: '4px 11px', border: '1px solid #d9d9d9', borderRadius: 6, minHeight: 76, maxHeight: 152, overflowY: 'auto' }}>
                                    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                        {form.getFieldValue('description') || '*No content provided*'}
                                    </ReactMarkdown>
                                </div>
                            )}
                        </Form.Item>
                        <Form.Item name="description" noStyle hidden={editPreviewMode}>
                            <Input.TextArea rows={3} placeholder={t('services.description_placeholder')} />
                        </Form.Item>
                    </Form.Item>
                </Form>
            </Modal>
            <Modal
                title={detailService?.name}
                open={detailOpen}
                onCancel={() => setDetailOpen(false)}
                footer={[
                    <Button key="close" onClick={() => setDetailOpen(false)}>
                        {t('button.close', 'Close')}
                    </Button>
                ]}
                destroyOnHidden
                width={800}
            >
                {detailService?.description ? (
                    <div className="markdown-preview" style={{ padding: '16px', background: '#fafafa', borderRadius: 8, marginTop: 16 }}>
                        <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                            {detailService.description}
                        </ReactMarkdown>
                    </div>
                ) : (
                    <div style={{ padding: '24px', textAlign: 'center', color: '#999' }}>
                        *No description provided*
                    </div>
                )}
            </Modal>
        </div>
    );
}
