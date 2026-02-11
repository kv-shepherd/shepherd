'use client';

/**
 * Services page — lists services across all systems.
 *
 * OpenAPI: GET /systems/{system_id}/services, POST /systems/{system_id}/services
 * ADR-0015: Service → System relationship (never directly to VM).
 */
import { useState } from 'react';
import {
    Table,
    Button,
    Space,
    Typography,
    Modal,
    Popconfirm,
    Form,
    Input,
    Select,
    message,
    Card,
    Tag,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    PlusOutlined,
    ReloadOutlined,
    CloudOutlined,
    DeleteOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type Service = components['schemas']['Service'];
type ServiceList = components['schemas']['ServiceList'];
type ServiceCreateRequest = components['schemas']['ServiceCreateRequest'];
type SystemList = components['schemas']['SystemList'];

export default function ServicesPage() {
    const { t } = useTranslation('common');
    const [messageApi, contextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [selectedSystemId, setSelectedSystemId] = useState<string>('');
    const [page, setPage] = useState(1);
    const [pageSize, setPageSize] = useState(20);
    const [form] = Form.useForm<ServiceCreateRequest & { system_id: string }>();

    // Fetch all systems for the system selector
    const { data: systemsData } = useApiGet<SystemList>(
        ['systems', 'all'],
        () => api.GET('/systems', { params: { query: { per_page: 100 } } })
    );

    // Fetch services for selected system (or first system by default)
    const activeSystemId = selectedSystemId || systemsData?.items?.[0]?.id || '';

    const { data: servicesData, isLoading, refetch } = useApiGet<ServiceList>(
        ['services', activeSystemId, page, pageSize],
        () => api.GET('/systems/{system_id}/services', {
            params: {
                path: { system_id: activeSystemId },
                query: { page, per_page: pageSize },
            },
        }),
        { enabled: !!activeSystemId }
    );

    // Create service mutation
    const createMutation = useApiMutation<
        { system_id: string; body: ServiceCreateRequest },
        Service
    >(
        ({ system_id, body }) => api.POST('/systems/{system_id}/services', {
            params: { path: { system_id } },
            body,
        }),
        {
            invalidateKeys: [['services']],
            onSuccess: () => {
                messageApi.success(t('message.success'));
                setCreateOpen(false);
                form.resetFields();
            },
            onError: (err) => {
                messageApi.error(err.code === 'CONFLICT' ? t('services.error.name_exists') : t('message.error'));
            },
        }
    );

    // Delete service mutation — requires confirm=true (ADR-0015 §13)
    const deleteMutation = useApiMutation<
        { systemId: string; serviceId: string },
        unknown
    >(
        ({ systemId, serviceId }) => api.DELETE('/systems/{system_id}/services/{service_id}', {
            params: {
                path: { system_id: systemId, service_id: serviceId },
                query: { confirm: true },
            },
        }),
        {
            invalidateKeys: [['services']],
            onSuccess: () => messageApi.success(t('message.success')),
            onError: (err) => messageApi.error(err.message || t('message.error')),
        }
    );

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
            width: 100,
            render: (_, record) => (
                <Popconfirm
                    title={t('message.confirm_delete')}
                    onConfirm={() => deleteMutation.mutate({
                        systemId: record.system_id,
                        serviceId: record.id,
                    })}
                    okText={t('button.confirm')}
                    cancelText={t('button.cancel')}
                >
                    <Button
                        type="text"
                        size="small"
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
            const { system_id, ...body } = values;
            createMutation.mutate({ system_id, body });
        });
    };

    return (
        <div>
            {contextHolder}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('nav.services')}</Title>
                    <Text type="secondary">{t('services.subtitle')}</Text>
                </div>
                <Space>
                    <Select
                        style={{ width: 200 }}
                        placeholder={t('services.select_system')}
                        value={activeSystemId || undefined}
                        onChange={(val) => { setSelectedSystemId(val); setPage(1); }}
                        options={systemsData?.items?.map((s) => ({
                            label: s.name,
                            value: s.id,
                        }))}
                    />
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('button.refresh')}
                    </Button>
                    <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                        {t('button.create')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<Service>
                    columns={columns}
                    dataSource={servicesData?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={{
                        current: page,
                        pageSize,
                        total: servicesData?.pagination?.total ?? 0,
                        showTotal: (total) => t('table.total', { total }),
                        onChange: (p, ps) => { setPage(p); setPageSize(ps); },
                    }}
                    size="middle"
                />
            </Card>

            {/* Create Service Modal */}
            <Modal
                title={`${t('button.create')} ${t('nav.services')}`}
                open={createOpen}
                onOk={handleCreate}
                onCancel={() => { setCreateOpen(false); form.resetFields(); }}
                confirmLoading={createMutation.isPending}
                forceRender
            >
                <Form form={form} layout="vertical" name="create-service">
                    <Form.Item
                        name="system_id"
                        label={t('nav.systems')}
                        rules={[{ required: true, message: t('services.validation.system_required') }]}
                        initialValue={activeSystemId}
                    >
                        <Select
                            placeholder={t('services.select_system')}
                            options={systemsData?.items?.map((s) => ({
                                label: s.name,
                                value: s.id,
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
        </div>
    );
}
