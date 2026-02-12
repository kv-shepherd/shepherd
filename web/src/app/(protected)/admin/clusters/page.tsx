'use client';

/**
 * Clusters management — admin page with real API calls.
 *
 * OpenAPI: GET/POST /admin/clusters
 * ADR-0012: Kubeconfig stored encrypted, K8s calls outside DB transactions.
 */
import { useState } from 'react';
import {
    Table,
    Button,
    Space,
    Typography,
    Tag,
    Badge,
    message,
    Modal,
    Form,
    Input,
    Card,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    PlusOutlined,
    ReloadOutlined,
    ClusterOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import { useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

const { Title, Text } = Typography;

type Cluster = components['schemas']['Cluster'];
type ClusterList = components['schemas']['ClusterList'];
type ClusterCreateRequest = components['schemas']['ClusterCreateRequest'];

const CLUSTER_STATUS_MAP: Record<string, { color: string; badge: 'success' | 'error' | 'warning' | 'default' }> = {
    HEALTHY: { color: 'green', badge: 'success' },
    UNHEALTHY: { color: 'red', badge: 'error' },
    UNREACHABLE: { color: 'orange', badge: 'warning' },
    UNKNOWN: { color: 'default', badge: 'default' },
};

export default function ClustersPage() {
    const { t } = useTranslation(['admin', 'common']);
    const [messageApi, contextHolder] = message.useMessage();
    const [createOpen, setCreateOpen] = useState(false);
    const [form] = Form.useForm<ClusterCreateRequest>();

    // Fetch clusters
    const { data, isLoading, refetch } = useApiGet<ClusterList>(
        ['admin-clusters'],
        () => api.GET('/admin/clusters')
    );

    // Create cluster mutation
    const createMutation = useApiMutation<ClusterCreateRequest, Cluster>(
        (req) => api.POST('/admin/clusters', { body: req }),
        {
            invalidateKeys: [['admin-clusters']],
            onSuccess: () => {
                messageApi.success(t('common:message.success'));
                setCreateOpen(false);
                form.resetFields();
            },
            onError: (err) => messageApi.error(err.message || t('common:message.error')),
        }
    );

    const columns: ColumnsType<Cluster> = [
        {
            title: t('common:table.name'),
            dataIndex: 'display_name',
            key: 'name',
            render: (displayName: string, record: Cluster) => (
                <Space>
                    <ClusterOutlined style={{ color: '#1677ff' }} />
                    <div>
                        <Text strong>{displayName ?? record.name}</Text>
                        <br />
                        <Text type="secondary" style={{ fontSize: 12 }}>
                            {record.name}
                        </Text>
                    </div>
                </Space>
            ),
        },
        {
            title: t('common:table.status'),
            dataIndex: 'status',
            key: 'status',
            width: 140,
            render: (status: Cluster['status']) => {
                const config = CLUSTER_STATUS_MAP[status] ?? CLUSTER_STATUS_MAP.UNKNOWN;
                return <Badge status={config.badge} text={<Tag color={config.color}>{status}</Tag>} />;
            },
        },
        {
            title: t('clusters.kubevirt_version'),
            dataIndex: 'kubevirt_version',
            key: 'kubevirt_version',
            width: 130,
            render: (v: string | undefined) => v ? <Tag color="blue">KV {v}</Tag> : '—',
        },
        {
            title: t('clusters.enabled'),
            dataIndex: 'enabled',
            key: 'enabled',
            width: 90,
            render: (enabled: boolean) => (
                <Tag color={enabled ? 'green' : 'default'}>
                    {enabled ? 'Yes' : 'No'}
                </Tag>
            ),
        },
        {
            title: t('clusters.api_server'),
            dataIndex: 'api_server_url',
            key: 'api_server_url',
            ellipsis: true,
            render: (url: string) => <Text type="secondary" copyable>{url}</Text>,
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => (
                <Text type="secondary">{date ? dayjs(date).format('YYYY-MM-DD HH:mm') : '—'}</Text>
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
            <div style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 24,
            }}>
                <div>
                    <Title level={4} style={{ margin: 0 }}>{t('clusters.title')}</Title>
                    <Text type="secondary">{t('clusters.subtitle')}</Text>
                </div>
                <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        onClick={() => setCreateOpen(true)}
                    >
                        {t('clusters.add')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<Cluster>
                    columns={columns}
                    dataSource={data?.items ?? []}
                    rowKey="id"
                    loading={isLoading}
                    pagination={{
                        total: data?.pagination?.total ?? 0,
                        pageSize: 20,
                        showTotal: (total) => t('common:table.total', { total }),
                    }}
                    size="middle"
                />
            </Card>

            {/* Add Cluster Modal */}
            <Modal
                title={t('clusters.add')}
                open={createOpen}
                onOk={handleCreate}
                onCancel={() => { setCreateOpen(false); form.resetFields(); }}
                confirmLoading={createMutation.isPending}
                forceRender
            >
                <Form form={form} layout="vertical" name="create-cluster">
                    <Form.Item
                        name="name"
                        label={t('common:table.name')}
                        rules={[{ required: true, message: 'Cluster name is required' }]}
                    >
                        <Input placeholder="e.g. cluster-prod-01" />
                    </Form.Item>
                    <Form.Item name="display_name" label="Display Name">
                        <Input placeholder="e.g. Production Cluster" />
                    </Form.Item>
                    <Form.Item
                        name="kubeconfig"
                        label="Kubeconfig (Base64)"
                        rules={[{ required: true, message: 'Kubeconfig is required' }]}
                        extra="Base64-encoded kubeconfig (stored encrypted, ADR-0012)"
                    >
                        <Input.TextArea
                            rows={6}
                            placeholder="Paste base64-encoded kubeconfig content..."
                        />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
