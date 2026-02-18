'use client';

/**
 * Systems page — CRUD management with real API calls.
 *
 * OpenAPI: GET/POST /systems, GET/DELETE /systems/{system_id}
 * ADR-0019: RFC 1035 naming (max 15 chars, lowercase alphanumeric + hyphen)
 * ADR-0015: System is top-level entity (System → Service → VM)
 */
import { useState } from 'react';
import {
    Table,
    Button,
    Space,
    Typography,
    Modal,
    Form,
    Input,
    message,
    Popconfirm,
    Card,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    PlusOutlined,
    ReloadOutlined,
    DeleteOutlined,
    AppstoreOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { useApiGet, useApiMutation, useApiAction } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type System = components['schemas']['System'];
type SystemList = components['schemas']['SystemList'];
type SystemCreateRequest = components['schemas']['SystemCreateRequest'];

/** RFC 1035 label validation */
const RFC1035_PATTERN = /^[a-z]([a-z0-9-]*[a-z0-9])?$/;

export default function SystemsPage() {
    const { t } = useTranslation('common');
    const [messageApi, contextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [form] = Form.useForm<SystemCreateRequest>();
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);

    // Fetch systems list
    const { data, isLoading, refetch } = useApiGet<SystemList>(
        ['systems', page, pageSize],
        () => api.GET('/systems', {
            params: { query: { page, per_page: pageSize } },
        })
    );

    // Create system mutation
    const createMutation = useApiMutation<SystemCreateRequest, System>(
        (req) => api.POST('/systems', { body: req }),
        {
            invalidateKeys: [['systems']],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                setCreateOpen(false);
                form.resetFields();
            },
            onError: (err) => {
                messageApi.error(err.code === 'CONFLICT' ? 'System name already exists' : t('message.error'));
            },
        }
    );

    // Delete system action
    const deleteMutation = useApiAction<{ systemId: string; confirmName: string }>(
        ({ systemId, confirmName }) => api.DELETE('/systems/{system_id}', {
            params: {
                path: { system_id: systemId },
                query: { confirm_name: confirmName },
            },
        }),
        {
            invalidateKeys: [['systems']],
            onSuccess: () => messageApi.success(t('message.success')),
            onError: () => messageApi.error(t('message.error')),
        }
    );

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
            width: 120,
            render: (_, record) => (
                <Popconfirm
                    title={t('message.confirm_delete')}
                    onConfirm={() => deleteMutation.mutate({ systemId: record.id, confirmName: record.name })}
                    okText={t('button.confirm')}
                    cancelText={t('button.cancel')}
                >
                    <Button
                        type="text"
                        danger
                        icon={<DeleteOutlined />}
                        loading={deleteMutation.isPending}
                    />
                </Popconfirm>
            ),
        },
    ];

    const handleCreate = () => {
        form.validateFields().then((values) => {
            createMutation.mutate(values);
        });
    };

    return (
        <div>
            {contextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('nav.systems')}</Title>
                    <Text type="secondary">Manage system hierarchy (ADR-0015)</Text>
                </div>
                <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('button.refresh')}
                    </Button>
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                        {t('button.create')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<System>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={{
                        current: page,
                        pageSize,
                        total: data?.pagination?.total ?? 0,
                        showTotal: (total) => t('table.total', { total }),
                        onChange: (p, ps) => { setPage(p); setPageSize(ps); },
                    }}
                    size="middle"
                />
            </Card>

            {/* Create System Modal */}
            <Modal
                title={`${t('button.create')} System`}
                open={createOpen}
                onOk={handleCreate}
                onCancel={() => { setCreateOpen(false); form.resetFields(); }}
                confirmLoading={createMutation.isPending}
                forceRender
            >
                <Form form={form} layout="vertical" name="create-system">
                    <Form.Item
                        name="name"
                        label={t('table.name')}
                        rules={[
                            { required: true, message: 'System name is required' },
                            { max: 15, message: 'Max 15 characters' },
                            {
                                pattern: RFC1035_PATTERN,
                                message: 'Must start with lowercase letter, contain only a-z, 0-9, hyphens',
                            },
                        ]}
                    >
                        <Input placeholder="e.g. shop, hr-system" maxLength={15} />
                    </Form.Item>
                    <Form.Item name="description" label={t('table.description')}>
                        <Input.TextArea rows={3} placeholder="Optional description" />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
