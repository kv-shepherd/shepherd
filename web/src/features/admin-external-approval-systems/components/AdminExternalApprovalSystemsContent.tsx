'use client';

import { useMemo, useState } from 'react';
import {
    Alert,
    App,
    Button,
    Col,
    Form,
    Input,
    InputNumber,
    Modal,
    Popconfirm,
    Row,
    Space,
    Switch,
    Table,
    Tag,
    Tooltip,
    Typography,
} from 'antd';
import type { TableProps } from 'antd';
import {
    ApiOutlined,
    CheckCircleOutlined,
    DeleteOutlined,
    EditOutlined,
    KeyOutlined,
    PlusOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SummaryMetricCard } from '@/components/feedback/SummaryMetricCard';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { useApiAction, useApiGet, useApiMutation } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { translateApiError } from '@/lib/api/errorMessage';
import type { components } from '@/types/api.gen';

type ExternalApprovalSystem = components['schemas']['ExternalApprovalSystem'];
type ExternalApprovalSystemCreateRequest = components['schemas']['ExternalApprovalSystemCreateRequest'];
type ExternalApprovalSystemUpdateRequest = components['schemas']['ExternalApprovalSystemUpdateRequest'];

type FormValues = {
    name: string;
    enabled: boolean;
    webhook_url: string;
    webhook_headers?: string;
    timeout_seconds: number;
    retry_count: number;
    retry_backoff_seconds: number;
    signing_key?: string;
    sort_order: number;
};

type UpdateMutationVars = {
    id: string;
    body: ExternalApprovalSystemUpdateRequest;
};

const EXTERNAL_APPROVAL_QUERY_KEY = ['admin-external-approval-systems'];

const defaultFormValues: FormValues = {
    name: '',
    enabled: true,
    webhook_url: '',
    webhook_headers: 'X-Shepherd-Source: shepherd',
    timeout_seconds: 30,
    retry_count: 3,
    retry_backoff_seconds: 2,
    signing_key: '',
    sort_order: 0,
};

const { Text } = Typography;

export function AdminExternalApprovalSystemsContent() {
    const { t } = useTranslation(['admin', 'common', 'errors']);
    const { message } = App.useApp();
    const [form] = Form.useForm<FormValues>();
    const [modalOpen, setModalOpen] = useState(false);
    const [editingSystem, setEditingSystem] = useState<ExternalApprovalSystem | null>(null);

    const systemsQuery = useApiGet(
        EXTERNAL_APPROVAL_QUERY_KEY,
        () => api.GET('/admin/external-approval-systems'),
    );
    const systems = useMemo(() => systemsQuery.data?.items ?? [], [systemsQuery.data?.items]);
    const activeCandidate = useMemo(() => {
        return [...systems]
            .filter((system) => system.enabled)
            .sort((a, b) => ((a.sort_order ?? 0) - (b.sort_order ?? 0)) || a.name.localeCompare(b.name))[0];
    }, [systems]);

    const createMutation = useApiMutation<ExternalApprovalSystemCreateRequest, ExternalApprovalSystem>(
        (body) => api.POST('/admin/external-approval-systems', { body }),
        {
            invalidateKeys: [EXTERNAL_APPROVAL_QUERY_KEY],
            onSuccess: () => {
                message.success(t('externalApprovalSystems.message.created'));
                closeModal();
            },
            onError: (error) => message.error(translateApiError(t, error)),
        },
    );
    const updateMutation = useApiMutation<UpdateMutationVars, ExternalApprovalSystem>(
        ({ id, body }) => api.PATCH('/admin/external-approval-systems/{system_id}', {
            params: { path: { system_id: id } },
            body,
        }),
        {
            invalidateKeys: [EXTERNAL_APPROVAL_QUERY_KEY],
            onSuccess: () => {
                message.success(t('externalApprovalSystems.message.updated'));
                closeModal();
            },
            onError: (error) => message.error(translateApiError(t, error)),
        },
    );
    const deleteMutation = useApiAction<{ id: string }>(
        ({ id }) => api.DELETE('/admin/external-approval-systems/{system_id}', {
            params: { path: { system_id: id } },
        }),
        {
            invalidateKeys: [EXTERNAL_APPROVAL_QUERY_KEY],
            onSuccess: () => message.success(t('externalApprovalSystems.message.deleted')),
            onError: (error) => message.error(translateApiError(t, error)),
        },
    );

    const openCreateModal = () => {
        setEditingSystem(null);
        form.resetFields();
        form.setFieldsValue(defaultFormValues);
        setModalOpen(true);
    };

    const openEditModal = (system: ExternalApprovalSystem) => {
        setEditingSystem(system);
        form.resetFields();
        form.setFieldsValue({
            name: system.name,
            enabled: system.enabled,
            webhook_url: system.webhook_url,
            webhook_headers: formatHeaders(system.webhook_headers),
            timeout_seconds: system.timeout_seconds,
            retry_count: system.retry_count,
            retry_backoff_seconds: system.retry_backoff_seconds,
            signing_key: '',
            sort_order: system.sort_order ?? 0,
        });
        setModalOpen(true);
    };

    const closeModal = () => {
        setModalOpen(false);
        setEditingSystem(null);
        form.resetFields();
    };

    const handleSubmit = async () => {
        const values = await form.validateFields();
        let headers: Record<string, string>;
        try {
            headers = parseHeaders(values.webhook_headers, {
                line: t('externalApprovalSystems.form.headers_invalid_line'),
                name: t('externalApprovalSystems.form.headers_invalid_name'),
            });
        } catch (error) {
            form.setFields([
                {
                    name: 'webhook_headers',
                    errors: [error instanceof Error ? error.message : t('externalApprovalSystems.form.headers_invalid')],
                },
            ]);
            return;
        }

        if (editingSystem) {
            const body: ExternalApprovalSystemUpdateRequest = {
                name: values.name.trim(),
                enabled: values.enabled,
                webhook_url: values.webhook_url.trim(),
                webhook_headers: headers,
                timeout_seconds: values.timeout_seconds,
                retry_count: values.retry_count,
                retry_backoff_seconds: values.retry_backoff_seconds,
                sort_order: values.sort_order,
            };
            const signingKey = values.signing_key?.trim();
            if (signingKey) {
                body.signing_key = signingKey;
            }
            await updateMutation.mutateAsync({ id: editingSystem.id, body });
            return;
        }

        const signingKey = values.signing_key?.trim() ?? '';
        const body: ExternalApprovalSystemCreateRequest = {
            name: values.name.trim(),
            type: 'webhook',
            enabled: values.enabled,
            webhook_url: values.webhook_url.trim(),
            webhook_headers: headers,
            timeout_seconds: values.timeout_seconds,
            retry_count: values.retry_count,
            retry_backoff_seconds: values.retry_backoff_seconds,
            signing_key: signingKey,
            sort_order: values.sort_order,
        };
        await createMutation.mutateAsync(body);
    };

    const handleModalOk = () => {
        void handleSubmit().catch(() => undefined);
    };

    const columns: TableProps<ExternalApprovalSystem>['columns'] = [
        {
            title: t('externalApprovalSystems.table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (_, system) => (
                <Space direction="vertical" size={2}>
                    <Text strong>{system.name}</Text>
                    <Text type="secondary" style={{ maxWidth: 360 }} ellipsis={{ tooltip: system.id }}>
                        {system.id}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('externalApprovalSystems.table.status'),
            dataIndex: 'enabled',
            key: 'enabled',
            width: 120,
            render: (enabled: boolean) => (
                <Tag color={enabled ? 'green' : 'default'}>
                    {enabled ? t('externalApprovalSystems.status.enabled') : t('externalApprovalSystems.status.disabled')}
                </Tag>
            ),
        },
        {
            title: t('externalApprovalSystems.table.endpoint'),
            dataIndex: 'webhook_url',
            key: 'webhook_url',
            render: (value: string) => (
                <Text copyable={{ text: value }} style={{ maxWidth: 360 }} ellipsis={{ tooltip: value }}>
                    {value}
                </Text>
            ),
        },
        {
            title: t('externalApprovalSystems.table.delivery'),
            key: 'delivery',
            width: 210,
            render: (_, system) => (
                <Space direction="vertical" size={2}>
                    <Text>{t('externalApprovalSystems.table.timeout_value', { seconds: system.timeout_seconds })}</Text>
                    <Text type="secondary">
                        {t('externalApprovalSystems.table.retry_value', {
                            count: system.retry_count,
                            seconds: system.retry_backoff_seconds,
                        })}
                    </Text>
                </Space>
            ),
        },
        {
            title: t('externalApprovalSystems.table.protection'),
            key: 'protection',
            width: 160,
            render: (_, system) => (
                <Space size={6} wrap>
                    <Tag color={system.signing_key_set ? 'blue' : 'red'} icon={<KeyOutlined />}>
                        {system.signing_key_set
                            ? t('externalApprovalSystems.signing_key.set')
                            : t('externalApprovalSystems.signing_key.missing')}
                    </Tag>
                    <Tag>{t('externalApprovalSystems.table.headers_count', {
                        count: Object.keys(system.webhook_headers ?? {}).length,
                    })}</Tag>
                </Space>
            ),
        },
        {
            title: t('externalApprovalSystems.table.updated'),
            dataIndex: 'updated_at',
            key: 'updated_at',
            width: 180,
            render: (value?: string) => value ? dayjs(value).format('YYYY-MM-DD HH:mm') : '-',
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 120,
            render: (_, system) => (
                <Space size={4}>
                    <Tooltip title={t('common:button.edit')}>
                        <Button
                            aria-label={t('common:button.edit')}
                            icon={<EditOutlined />}
                            onClick={() => openEditModal(system)}
                        />
                    </Tooltip>
                    <Popconfirm
                        title={t('externalApprovalSystems.delete.confirm_title')}
                        description={t('externalApprovalSystems.delete.confirm_description', { name: system.name })}
                        okText={t('common:button.delete')}
                        cancelText={t('common:button.cancel')}
                        onConfirm={() => deleteMutation.mutate({ id: system.id })}
                    >
                        <Tooltip title={t('common:button.delete')}>
                            <Button
                                danger
                                aria-label={t('common:button.delete')}
                                icon={<DeleteOutlined />}
                                loading={deleteMutation.isPending}
                            />
                        </Tooltip>
                    </Popconfirm>
                </Space>
            ),
        },
    ];

    return (
        <div className="external-approval-systems-page copy-friendly-actions">
            <PageHeader
                title={t('externalApprovalSystems.title')}
                subtitle={t('externalApprovalSystems.subtitle')}
                actions={(
                    <Space wrap>
                        <Tooltip title={t('common:button.refresh')}>
                            <Button
                                aria-label={t('common:button.refresh')}
                                icon={<ReloadOutlined />}
                                onClick={() => void systemsQuery.refetch()}
                                loading={systemsQuery.isFetching}
                            />
                        </Tooltip>
                        <Button type="primary" icon={<PlusOutlined />} onClick={openCreateModal}>
                            {t('externalApprovalSystems.action.create')}
                        </Button>
                    </Space>
                )}
            />

            <Row gutter={[16, 16]} style={{ marginBottom: 18 }}>
                <Col xs={24} md={12} xl={6}>
                    <SummaryMetricCard
                        title={t('externalApprovalSystems.summary.total_title')}
                        value={systems.length}
                        description={t('externalApprovalSystems.summary.total_description')}
                        accentColor="#1677ff"
                        surfaceColor="#eef5ff"
                        visual={<ApiOutlined />}
                    />
                </Col>
                <Col xs={24} md={12} xl={6}>
                    <SummaryMetricCard
                        title={t('externalApprovalSystems.summary.enabled_title')}
                        value={systems.filter((system) => system.enabled).length}
                        description={t('externalApprovalSystems.summary.enabled_description')}
                        accentColor="#16a34a"
                        surfaceColor="#ecfdf3"
                        visual={<CheckCircleOutlined />}
                    />
                </Col>
                <Col xs={24} md={12} xl={6}>
                    <SummaryMetricCard
                        title={t('externalApprovalSystems.summary.active_title')}
                        value={activeCandidate?.name ?? t('externalApprovalSystems.summary.no_active')}
                        description={t('externalApprovalSystems.summary.active_description')}
                        accentColor="#0891b2"
                        surfaceColor="#ecfeff"
                        visual={<ApiOutlined />}
                    />
                </Col>
                <Col xs={24} md={12} xl={6}>
                    <SummaryMetricCard
                        title={t('externalApprovalSystems.summary.keys_title')}
                        value={systems.filter((system) => system.signing_key_set).length}
                        description={t('externalApprovalSystems.summary.keys_description')}
                        accentColor="#7c3aed"
                        surfaceColor="#f5f3ff"
                        visual={<KeyOutlined />}
                    />
                </Col>
            </Row>

            {systemsQuery.error ? (
                <Alert
                    showIcon
                    type="error"
                    message={t('externalApprovalSystems.load_failed')}
                    description={translateApiError(t, systemsQuery.error)}
                    style={{ marginBottom: 16 }}
                />
            ) : null}

            <PageSurface flush={true}>
                <Table
                    rowKey="id"
                    loading={systemsQuery.isLoading}
                    columns={columns}
                    dataSource={systems}
                    pagination={{ pageSize: 10, showSizeChanger: true }}
                    scroll={{ x: 1080 }}
                    locale={{
                        emptyText: (
                            <ActionEmptyState
                                title={t('externalApprovalSystems.empty_title')}
                                description={t('externalApprovalSystems.empty_description')}
                                actions={(
                                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreateModal}>
                                        {t('externalApprovalSystems.action.create')}
                                    </Button>
                                )}
                                compact={true}
                            />
                        ),
                    }}
                />
            </PageSurface>

            <Modal
                open={modalOpen}
                title={editingSystem ? t('externalApprovalSystems.modal.edit_title') : t('externalApprovalSystems.modal.create_title')}
                okText={editingSystem ? t('common:button.save') : t('common:button.create')}
                cancelText={t('common:button.cancel')}
                onCancel={closeModal}
                onOk={handleModalOk}
                confirmLoading={createMutation.isPending || updateMutation.isPending}
                destroyOnHidden={true}
                width={720}
            >
                <Form
                    form={form}
                    layout="vertical"
                    initialValues={defaultFormValues}
                    requiredMark={false}
                    style={{ marginTop: 16 }}
                >
                    <Row gutter={16}>
                        <Col xs={24} md={14}>
                            <Form.Item
                                name="name"
                                label={t('externalApprovalSystems.form.name')}
                                rules={[{ required: true, whitespace: true, message: t('externalApprovalSystems.form.required') }]}
                            >
                                <Input autoComplete="off" />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={10}>
                            <Form.Item
                                name="enabled"
                                label={t('externalApprovalSystems.form.enabled')}
                                valuePropName="checked"
                            >
                                <Switch />
                            </Form.Item>
                        </Col>
                    </Row>
                    <Form.Item
                        name="webhook_url"
                        label={t('externalApprovalSystems.form.webhook_url')}
                        rules={[
                            { required: true, whitespace: true, message: t('externalApprovalSystems.form.required') },
                            { type: 'url', message: t('externalApprovalSystems.form.url_invalid') },
                            {
                                validator: (_, value: string | undefined) => {
                                    const raw = value?.trim();
                                    if (!raw) {
                                        return Promise.resolve();
                                    }
                                    try {
                                        if (new URL(raw).protocol === 'https:') {
                                            return Promise.resolve();
                                        }
                                    } catch {
                                        return Promise.resolve();
                                    }
                                    return Promise.reject(new Error(t('externalApprovalSystems.form.url_https_required')));
                                },
                            },
                        ]}
                    >
                        <Input inputMode="url" autoComplete="off" />
                    </Form.Item>
                    <Form.Item
                        name="webhook_headers"
                        label={t('externalApprovalSystems.form.webhook_headers')}
                    >
                        <Input.TextArea autoSize={{ minRows: 3, maxRows: 6 }} />
                    </Form.Item>
                    <Row gutter={16}>
                        <Col xs={24} md={8}>
                            <Form.Item
                                name="timeout_seconds"
                                label={t('externalApprovalSystems.form.timeout_seconds')}
                                rules={[{ required: true, message: t('externalApprovalSystems.form.required') }]}
                            >
                                <InputNumber min={1} max={120} style={{ width: '100%' }} />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={8}>
                            <Form.Item
                                name="retry_count"
                                label={t('externalApprovalSystems.form.retry_count')}
                                rules={[{ required: true, message: t('externalApprovalSystems.form.required') }]}
                            >
                                <InputNumber min={1} max={10} style={{ width: '100%' }} />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={8}>
                            <Form.Item
                                name="retry_backoff_seconds"
                                label={t('externalApprovalSystems.form.retry_backoff_seconds')}
                                rules={[{ required: true, message: t('externalApprovalSystems.form.required') }]}
                            >
                                <InputNumber min={1} max={60} style={{ width: '100%' }} />
                            </Form.Item>
                        </Col>
                    </Row>
                    <Row gutter={16}>
                        <Col xs={24} md={12}>
                            <Form.Item
                                name="signing_key"
                                label={t('externalApprovalSystems.form.signing_key')}
                                rules={editingSystem ? [] : [
                                    { required: true, whitespace: true, message: t('externalApprovalSystems.form.required') },
                                ]}
                                extra={editingSystem ? t('externalApprovalSystems.form.signing_key_edit_extra') : undefined}
                            >
                                <Input.Password autoComplete="new-password" />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                            <Form.Item
                                name="sort_order"
                                label={t('externalApprovalSystems.form.sort_order')}
                                rules={[{ required: true, message: t('externalApprovalSystems.form.required') }]}
                            >
                                <InputNumber style={{ width: '100%' }} />
                            </Form.Item>
                        </Col>
                    </Row>
                </Form>
            </Modal>
        </div>
    );
}

function parseHeaders(
    raw: string | undefined,
    messages: { line: string; name: string },
): Record<string, string> {
    const headers: Record<string, string> = {};
    for (const line of (raw ?? '').split(/\r?\n/)) {
        const trimmed = line.trim();
        if (!trimmed) {
            continue;
        }
        const separator = trimmed.indexOf(':');
        if (separator <= 0) {
            throw new Error(messages.line);
        }
        const name = trimmed.slice(0, separator).trim();
        const value = trimmed.slice(separator + 1).trim();
        if (!/^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/.test(name)) {
            throw new Error(messages.name);
        }
        headers[name] = value;
    }
    return headers;
}

function formatHeaders(headers: Record<string, string> | undefined): string {
    return Object.entries(headers ?? {})
        .map(([name, value]) => `${name}: ${value}`)
        .join('\n');
}
