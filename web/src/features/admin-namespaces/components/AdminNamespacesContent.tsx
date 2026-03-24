'use client';

import {
    Button,
    Form,
    Input,
    Modal,
    Select,
    Space,
    Switch,
    Table,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    DeleteOutlined,
    EditOutlined,
    ExclamationCircleOutlined,
    EyeOutlined,
    GlobalOutlined,
    PlusOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useRouter } from 'next/navigation';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import {
    NotificationInboxGlyph,
    QueueReviewGlyph,
    RequestsOverviewGlyph,
    ServiceWorkspaceGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import {
    buildDashboardSetupResumeHref,
    resolveNextSetupAction,
} from '@/features/setup-guide/flow';
import { useAutoOpenIntent } from '@/features/setup-guide/hooks/useAutoOpenIntent';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useAdminNamespacesController } from '../hooks/useAdminNamespacesController';
import { ENV_MAP, ENV_OPTIONS, type NamespaceRegistry } from '../types';

const { Text, Paragraph } = Typography;

export function AdminNamespacesContent() {
    const { t } = useTranslation(['admin', 'common']);
    const router = useRouter();
    const setupGuide = useSetupGuide();
    const namespaces = useAdminNamespacesController({
        t,
        onCreateSuccess: (_namespace, context) => {
            if (!context.isFirstNamespace) {
                return false;
            }
            const nextAction = resolveNextSetupAction(setupGuide, 'namespace');
            if (!nextAction) {
                return false;
            }
            router.push(buildDashboardSetupResumeHref(nextAction));
            return true;
        },
    });

    useAutoOpenIntent('create-namespace', () => {
        namespaces.openCreateModal();
    });
    const namespaceItems = namespaces.data?.items ?? [];
    const namespaceSummary = {
        total: namespaceItems.length,
        prod: namespaceItems.filter((item) => item.environment === 'prod').length,
        enabled: namespaceItems.filter((item) => item.enabled).length,
        filtered: namespaceItems.length,
    };

    const columns: ColumnsType<NamespaceRegistry> = [
        {
            title: t('namespaces.table.namespace', 'Namespace'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string, record: NamespaceRegistry) => (
                <Space>
                    <GlobalOutlined style={{ color: '#1677ff' }} />
                    <div>
                        <Text strong>{name}</Text>
                        {record.description && (
                            <>
                                <br />
                                <Text type="secondary" style={{ fontSize: 12 }}>
                                    {record.description}
                                </Text>
                            </>
                        )}
                    </div>
                </Space>
            ),
        },
        {
            title: t('namespaces.environment'),
            dataIndex: 'environment',
            key: 'environment',
            width: 130,
            render: (env: string) => {
                const config = ENV_MAP[env] ?? { color: 'default', label: env };
                return <Tag color={config.color}>{config.label}</Tag>;
            },
        },
        {
            title: t('namespaces.enabled'),
            dataIndex: 'enabled',
            key: 'enabled',
            width: 90,
            render: (enabled: boolean) => (
                <Tag color={enabled ? 'green' : 'default'}>
                    {enabled ? t('namespaces.enabled_yes') : t('namespaces.enabled_no')}
                </Tag>
            ),
        },
        {
            title: t('common:table.created_by'),
            dataIndex: 'created_by',
            key: 'created_by',
            width: 180,
            render: (actor: string) => (
                <Space direction="vertical" size={0}>
                    <Text strong>{actor || '—'}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                        {t('namespaces.created_by_hint', 'Registry author')}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('common:table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => <LocalDateTimeText value={date} />,
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 220,
            render: (_: unknown, record: NamespaceRegistry) => (
                <Space size={4} wrap>
                    <Button
                        type="link"
                        size="small"
                        icon={<EyeOutlined />}
                        data-testid={`namespace-action-detail-${record.id}`}
                        onClick={() => router.push(`/admin/namespaces/${record.id}`)}
                    >
                        {t('common:button.detail')}
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        icon={<EditOutlined />}
                        data-testid={`namespace-action-edit-${record.id}`}
                        onClick={() => namespaces.openEditModal(record)}
                    >
                        {t('common:button.edit')}
                    </Button>
                    <Button
                        type="link"
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        data-testid={`namespace-action-delete-${record.id}`}
                        onClick={() => namespaces.openDeleteModal(record)}
                    >
                        {t('common:button.delete')}
                    </Button>
                </Space>
            ),
        },
    ];

    const handleCreate = () => {
        void namespaces.submitCreate();
    };

    const handleUpdate = () => {
        void namespaces.submitUpdate();
    };

    return (
        <div data-testid="admin-namespaces-page">
            {namespaces.messageContextHolder}
            <PageHeader
                title={t('namespaces.title')}
                subtitle={t('namespaces.subtitle')}
                actions={(
                    <Space>
                    <Select
                        placeholder={t('namespaces.filter_env')}
                        allowClear
                        style={{ width: 160 }}
                        data-testid="namespaces-env-filter"
                        value={namespaces.envFilter || undefined}
                        onChange={namespaces.changeEnvFilter}
                        options={ENV_OPTIONS.map((item) => ({ ...item }))}
                    />
                    <Button icon={<ReloadOutlined />} data-testid="namespaces-refresh-btn" onClick={() => namespaces.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <Button
                        type="primary"
                        icon={<PlusOutlined />}
                        data-testid="namespace-create-button"
                        onClick={namespaces.openCreateModal}
                    >
                        {t('namespaces.add')}
                    </Button>
                    </Space>
                )}
            />

            <div className="summary-card-grid">
                <SummaryMetricCard
                    title={t('namespaces.summary.total_title', 'Registered namespaces')}
                    value={namespaceSummary.total}
                    description={t('namespaces.summary.total_description', 'Logical namespaces currently available for placement and governance rules.')}
                    visual={<ServiceWorkspaceGlyph className="summary-metric-card__art" />}
                    accentColor="#1D5BFF"
                    surfaceColor="#E6F4FF"
                />
                <SummaryMetricCard
                    title={t('namespaces.summary.prod_title', 'Production')}
                    value={namespaceSummary.prod}
                    description={t('namespaces.summary.prod_description', 'Namespaces tagged for production requests and approvals.')}
                    visual={<QueueReviewGlyph className="summary-metric-card__art" />}
                    accentColor="#D66A1F"
                    surfaceColor="#FFF4E5"
                />
                <SummaryMetricCard
                    title={t('namespaces.summary.enabled_title', 'Enabled')}
                    value={namespaceSummary.enabled}
                    description={t('namespaces.summary.enabled_description', 'Namespaces currently available for new request placement.')}
                    visual={<RequestsOverviewGlyph className="summary-metric-card__art" />}
                    accentColor="#0F8F57"
                    surfaceColor="#E8FFF2"
                />
                <SummaryMetricCard
                    title={t('namespaces.summary.filtered_title', 'Visible now')}
                    value={namespaceSummary.filtered}
                    description={t('namespaces.summary.filtered_description', 'Entries matching the current environment filter.')}
                    visual={<NotificationInboxGlyph className="summary-metric-card__art" />}
                    accentColor="#6D4DE3"
                    surfaceColor="#F5EDFF"
                />
            </div>

            <PageSurface flush={true}>
                <Table<NamespaceRegistry>
                    columns={columns}
                    dataSource={namespaceItems}
                    rowKey="id"
                    loading={namespaces.isLoading}
                    pagination={{
                        current: namespaces.page,
                        total: namespaces.data?.pagination?.total ?? 0,
                        pageSize: 20,
                        onChange: namespaces.setPage,
                        showTotal: (total) => t('common:table.total', { total }),
                    }}
                    size="middle"
                    locale={{
                        emptyText: (
                            <ActionEmptyState
                                compact={true}
                                title={t('namespaces.empty', 'No namespaces registered')}
                                description={t('namespaces.empty_description', 'Register the first logical namespace before users submit VM requests into a governed resource domain.')}
                                visual={<ServiceWorkspaceGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                            />
                        ),
                    }}
                />
            </PageSurface>

            <Modal
                title={t('namespaces.add')}
                open={namespaces.createOpen}
                onOk={handleCreate}
                onCancel={namespaces.closeCreateModal}
                confirmLoading={namespaces.createPending}
                forceRender={true}
                data-testid="namespace-create-modal"
            >
                    <Form form={namespaces.createForm} layout="vertical" name="create-namespace" preserve={false}>
                        <Form.Item
                            name="name"
                            label={t('common:table.name')}
                            rules={[
                                { required: true, message: t('namespaces.validation.name_required') },
                                { max: 63, message: t('namespaces.validation.name_max') },
                                {
                                    pattern: /^[a-z][a-z0-9-]*$/,
                                    message: t('namespaces.validation.name_format'),
                                },
                            ]}
                            extra={t('namespaces.name_hint')}
                        >
                            <Input placeholder="e.g. prod-shop, dev-analytics" />
                        </Form.Item>
                        <Form.Item
                            name="environment"
                            label={t('namespaces.environment')}
                            rules={[{ required: true, message: t('namespaces.validation.env_required') }]}
                            extra={t('namespaces.env_hint')}
                        >
                            <Select options={ENV_OPTIONS.map((item) => ({ ...item }))} />
                        </Form.Item>
                        <Form.Item
                            name="description"
                            label={t('common:table.description')}
                        >
                            <Input.TextArea rows={3} placeholder={t('namespaces.desc_placeholder')} />
                        </Form.Item>
                    </Form>
            </Modal>

            <Modal
                title={`${t('common:button.edit')}: ${namespaces.editingNs?.name ?? ''}`}
                open={namespaces.editOpen}
                onOk={handleUpdate}
                onCancel={namespaces.closeEditModal}
                confirmLoading={namespaces.updatePending}
                forceRender={true}
                data-testid="namespace-edit-modal"
            >
                    <Form form={namespaces.editForm} layout="vertical" name="edit-namespace" preserve={false}>
                        <Paragraph type="secondary" style={{ marginBottom: 16 }}>
                            {t('namespaces.edit_note')}
                        </Paragraph>
                        <Form.Item
                            name="description"
                            label={t('common:table.description')}
                        >
                            <Input.TextArea rows={3} />
                        </Form.Item>
                        <Form.Item
                            name="enabled"
                            label={t('namespaces.enabled')}
                            valuePropName="checked"
                        >
                            <Switch />
                        </Form.Item>
                    </Form>
            </Modal>

            <Modal
                title={(
                    <Space>
                        <ExclamationCircleOutlined style={{ color: '#ff4d4f' }} />
                        {t('namespaces.delete_title')}
                    </Space>
                )}
                open={namespaces.deleteOpen}
                onOk={namespaces.submitDelete}
                onCancel={namespaces.closeDeleteModal}
                confirmLoading={namespaces.deletePending}
                okButtonProps={{
                    danger: true,
                    disabled: namespaces.deleteConfirmName !== namespaces.deletingNs?.name,
                }}
                okText={t('common:button.delete')}
                data-testid="namespace-delete-modal"
            >
                    <Paragraph>
                        {t('namespaces.delete_confirm', { name: namespaces.deletingNs?.name })}
                    </Paragraph>
                    <Paragraph type="secondary">
                        {t('namespaces.delete_type_name')}
                    </Paragraph>
                    <Input
                        value={namespaces.deleteConfirmName}
                        onChange={(e) => namespaces.setDeleteConfirmName(e.target.value)}
                        placeholder={namespaces.deletingNs?.name}
                        data-testid="namespace-delete-confirm-input"
                        status={
                            namespaces.deleteConfirmName && namespaces.deleteConfirmName !== namespaces.deletingNs?.name
                                ? 'error'
                                : undefined
                        }
                    />
            </Modal>
        </div>
    );
}
