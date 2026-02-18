'use client';

import {
    Badge,
    Button,
    Card,
    Form,
    Input,
    Modal,
    Select,
    Space,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ClusterOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { useAdminClustersController } from '../hooks/useAdminClustersController';
import { CLUSTER_STATUS_MAP, type Cluster } from '../types';

const { Title, Text } = Typography;

export function AdminClustersContent() {
    const { t } = useTranslation(['admin', 'common']);
    const clusters = useAdminClustersController({ t });

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
            render: (version: string | undefined) => version ? <Tag color="blue">KV {version}</Tag> : '—',
        },
        {
            title: t('clusters.environment'),
            dataIndex: 'environment',
            key: 'environment',
            width: 150,
            render: (env: 'test' | 'prod' | undefined, record: Cluster) => (
                <Select
                    value={env ?? 'test'}
                    style={{ width: 120 }}
                    data-testid={`cluster-env-select-${record.id}`}
                    options={[
                        { value: 'test', label: t('clusters.env_test') },
                        { value: 'prod', label: t('clusters.env_prod') },
                    ]}
                    onChange={(nextEnv) => clusters.updateEnvironment(record.id, nextEnv as 'test' | 'prod')}
                    loading={clusters.updateEnvironmentPending}
                />
            ),
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

    return (
        <div data-testid="admin-clusters-page">
            {clusters.messageContextHolder}
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
                    <Button icon={<ReloadOutlined />} data-testid="clusters-refresh-btn" onClick={() => clusters.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button type="primary" icon={<PlusOutlined />} data-testid="cluster-create-button" onClick={clusters.openCreateModal}>
                        {t('clusters.add')}
                    </Button>
                </Space>
            </div>

            <Card style={{ borderRadius: 12 }} styles={{ body: { padding: 0 } }}>
                <Table<Cluster>
                    columns={columns}
                    dataSource={clusters.data?.items ?? []}
                    rowKey="id"
                    loading={clusters.isLoading}
                    pagination={{
                        total: clusters.data?.pagination?.total ?? 0,
                        pageSize: 20,
                        showTotal: (total) => t('common:table.total', { total }),
                    }}
                    size="middle"
                />
            </Card>

            <Modal
                title={t('clusters.add')}
                open={clusters.createOpen}
                onOk={() => {
                    void clusters.submitCreate();
                }}
                onCancel={clusters.closeCreateModal}
                confirmLoading={clusters.createPending}
                forceRender
                data-testid="cluster-create-modal"
            >
                <Form form={clusters.form} layout="vertical" name="create-cluster">
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
                        name="environment"
                        label={t('clusters.environment')}
                        initialValue="test"
                        rules={[{ required: true, message: t('clusters.environment_required') }]}
                    >
                        <Select
                            options={[
                                { value: 'test', label: t('clusters.env_test') },
                                { value: 'prod', label: t('clusters.env_prod') },
                            ]}
                        />
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
