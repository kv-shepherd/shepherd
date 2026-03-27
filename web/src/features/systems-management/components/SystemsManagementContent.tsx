'use client';

import React, { useMemo, useState } from 'react';
import {
    Button,
    Descriptions,
    Form,
    Input,
    Modal,
    Space,
    Table,
    Typography,
    Upload,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
    AppstoreOutlined,
    EditOutlined,
    DeleteOutlined,
    ExclamationCircleOutlined,
    EyeOutlined,
    PlusOutlined,
    ReloadOutlined,
    TeamOutlined,
    UploadOutlined,
    FileTextOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSanitize from 'rehype-sanitize';
import { useRouter, useSearchParams } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { PermissionGuard } from '@/components/auth/PermissionGuard';
import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import { SystemsOverviewGlyph } from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { WorkbenchDetailModal } from '@/components/workbench/WorkbenchDetailModal';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import {
    buildDashboardSetupResumeHref,
    resolveNextSetupAction,
} from '@/features/setup-guide/flow';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import { useAutoOpenIntent } from '@/features/setup-guide/hooks/useAutoOpenIntent';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useSystemsManagementController } from '../hooks/useSystemsManagementController';
import { RFC1035_PATTERN, type System } from '../types';
import { SystemMembersModal } from './SystemMembersModal';
import type { components } from '@/types/api.gen';

const { Text, Paragraph } = Typography;
type Service = components['schemas']['Service'];
type ServiceList = components['schemas']['ServiceList'];

interface SystemServicesCellProps {
    systemId: string;
    onOpenService: (service: Service) => void;
}

function SystemServicesCell({ systemId, onOpenService }: SystemServicesCellProps) {
    const { t } = useTranslation('common');
    const servicesQuery = useApiGet<ServiceList>(
        ['system-services-preview', systemId],
        () => api.GET('/systems/{system_id}/services', {
            params: {
                path: { system_id: systemId },
                query: { per_page: 100 },
            },
        }),
        { enabled: Boolean(systemId) },
    );
    const items = servicesQuery.data?.items ?? [];

    if (servicesQuery.isLoading) {
        return <Text type="secondary">{t('message.loading', 'Loading...')}</Text>;
    }

    if (items.length === 0) {
        return <Text type="secondary">{t('systems.related_services_empty', 'No services found for this system')}</Text>;
    }

    const previewItems = items.slice(0, 3);
    const remaining = items.length - previewItems.length;

    return (
        <Space direction="vertical" size={0}>
            {previewItems.map((service) => (
                <Button
                    key={service.id}
                    type="link"
                    size="small"
                    data-testid={`system-service-link-${service.id}`}
                    style={{ paddingInline: 0, justifyContent: 'flex-start' }}
                    onClick={() => onOpenService(service)}
                >
                    {service.name}
                </Button>
            ))}
            {remaining > 0 ? (
                <Text type="secondary" style={{ fontSize: 12 }}>
                    +{remaining} more
                </Text>
            ) : null}
        </Space>
    );
}

export function SystemsManagementContent() {
    const { t } = useTranslation('common');
    const router = useRouter();
    const searchParams = useSearchParams();
    const setupGuide = useSetupGuide();
    const systems = useSystemsManagementController({
        t,
        onCreateSuccess: (_system, context) => {
            if (!context.isFirstSystem) {
                return false;
            }
            const nextAction = resolveNextSetupAction(setupGuide, 'system');
            if (!nextAction) {
                return false;
            }
            router.push(buildDashboardSetupResumeHref(nextAction));
            return true;
        },
    });

    const [createPreviewMode, setCreatePreviewMode] = useState(false);
    const [editPreviewMode, setEditPreviewMode] = useState(false);
    const [detailOpen, setDetailOpen] = useState(false);
    const [detailSystem, setDetailSystem] = useState<System | null>(null);
    const [dismissedQueryDetailSystemId, setDismissedQueryDetailSystemId] = useState<string | null>(null);
    const detailSystemIdFromQuery = searchParams.get('detail_system_id') || undefined;

    const activeDetailSystem = useMemo(() => {
        if (!detailSystemIdFromQuery || detailSystemIdFromQuery === dismissedQueryDetailSystemId) {
            return detailSystem;
        }
        return systems.data?.items?.find((system) => system.id === detailSystemIdFromQuery) ?? detailSystem;
    }, [detailSystem, detailSystemIdFromQuery, dismissedQueryDetailSystemId, systems.data?.items]);

    const detailModalOpen = detailOpen || Boolean(activeDetailSystem);

    const closeDetailModal = () => {
        setDetailOpen(false);
        setDetailSystem(null);
        if (detailSystemIdFromQuery) {
            setDismissedQueryDetailSystemId(detailSystemIdFromQuery);
            const url = new URL(window.location.href);
            url.searchParams.delete('detail_system_id');
            const nextURL = `${url.pathname}${url.searchParams.toString() === '' ? '' : `?${url.searchParams.toString()}`}${url.hash}`;
            window.history.replaceState(window.history.state, '', nextURL);
        }
    };

    const relatedServicesQuery = useApiGet<ServiceList>(
        ['system-related-services', activeDetailSystem?.id],
        () => api.GET('/systems/{system_id}/services', {
            params: {
                path: { system_id: activeDetailSystem!.id },
                query: { per_page: 100 },
            },
        }),
        { enabled: detailModalOpen && Boolean(activeDetailSystem?.id) },
    );

    useAutoOpenIntent('create-system', () => {
        systems.openCreateModal();
    });

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
                <Text type="secondary"><LocalDateTimeText value={date} /></Text>
            ),
        },
        {
            title: t('systems.related_services_column', 'Services'),
            key: 'related_services',
            width: 220,
            render: (_, record) => (
                <SystemServicesCell
                    systemId={record.id}
                    onOpenService={(service) => {
                        router.push(`/services?system_id=${service.system_id}&detail_service_id=${service.id}`);
                    }}
                />
            ),
        },
        {
            title: t('table.actions'),
            key: 'actions',
            width: 200,
            render: (_, record) => (
                <Space>
                    <Button
                        type="link"
                        size="small"
                        data-testid={`system-action-detail-${record.id}`}
                        icon={<EyeOutlined />}
                        onClick={() => {
                            setDetailSystem(record);
                            setDetailOpen(true);
                            setDismissedQueryDetailSystemId(null);
                        }}
                    >
                        {t('button.detail', 'Details')}
                    </Button>
                    <PermissionGuard permission="rbac:manage">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`system-action-members-${record.id}`}
                            icon={<TeamOutlined />}
                            onClick={() => systems.openMembersModal(record)}
                        >
                            {t('button.manage_members')}
                        </Button>
                    </PermissionGuard>
                    <PermissionGuard permission="system:write">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`system-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            loading={systems.updatePending && systems.editingSystem?.id === record.id}
                            onClick={() => systems.openEditModal(record)}
                        >
                            {t('button.edit')}
                        </Button>
                    </PermissionGuard>
                    <PermissionGuard permission="system:delete">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`system-action-delete-${record.id}`}
                            danger
                            icon={<DeleteOutlined />}
                            loading={systems.deletePending && systems.deletingSystem?.id === record.id}
                            onClick={() => systems.openDeleteModal(record)}
                        >
                            {t('button.delete')}
                        </Button>
                    </PermissionGuard>
                </Space>
            ),
        },
    ];

    return (
        <div>
            {systems.messageContextHolder}
            <PageHeader
                title={t('nav.systems')}
                subtitle={t('systems.subtitle')}
                actions={(
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
                )}
            />

            {(systems.data?.items?.length ?? 0) === 0 && !systems.isLoading ? (
                <SetupGuideCard variant="systems" />
            ) : (
                <PageSurface flush={true}>
                    <Table<System>
                        columns={columns}
                        dataSource={systems.data?.items ?? []}
                        rowKey="id"
                        loading={systems.isLoading}
                        scroll={{ x: 'max-content' }}
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
                </PageSurface>
            )}

            <Modal
                title={t('systems.modal.create_title')}
                open={systems.createOpen}
                onOk={() => {
                    void systems.submitCreate();
                }}
                onCancel={systems.closeCreateModal}
                confirmLoading={systems.createPending}
                forceRender={true}
                data-testid="system-create-modal"
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
                                            systems.form.setFieldValue('description', text);
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
                            <Input.TextArea rows={3} placeholder={t('systems.description_placeholder')} />
                        </Form.Item>
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
                forceRender={true}
                data-testid="system-edit-modal"
            >
                <Form form={systems.editForm} layout="vertical" name="edit-system">
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
                                            systems.editForm.setFieldValue('description', text);
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
                            <Input.TextArea rows={3} placeholder={t('systems.edit_description_placeholder')} />
                        </Form.Item>
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
                forceRender={true}
                data-testid="system-delete-modal"
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

            <WorkbenchDetailModal
                title={activeDetailSystem?.name}
                open={detailModalOpen}
                onCancel={closeDetailModal}
                footer={[
                    <Button
                        key="open-services"
                        type="primary"
                        onClick={() => {
                            if (!activeDetailSystem) {
                                return;
                            }
                            closeDetailModal();
                            router.push(`/services?system_id=${activeDetailSystem.id}`);
                        }}
                    >
                        {t('systems.open_services', 'Open Services')}
                    </Button>,
                    <Button key="close" onClick={closeDetailModal}>
                        {t('button.close', 'Close')}
                    </Button>
                ]}
                forceRender
                width="min(1120px, calc(100vw - 16px))"
                contentMinWidth={1040}
            >
                <Space direction="vertical" size={16} className="workbench-detail-modal__stack">
                    <Descriptions
                        size="small"
                        column={{ xs: 1, sm: 1, md: 1, lg: 2, xl: 2, xxl: 2 }}
                    >
                        <Descriptions.Item label={t('table.name')}>
                            <Text strong>{activeDetailSystem?.name}</Text>
                        </Descriptions.Item>
                        <Descriptions.Item label={t('table.created_by')}>
                            {activeDetailSystem?.created_by || '—'}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('table.created_at')}>
                            {activeDetailSystem?.created_at ? <LocalDateTimeText value={activeDetailSystem.created_at} /> : '—'}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('systems.related_services_column', 'Services')}>
                            {relatedServicesQuery.data?.pagination?.total ?? relatedServicesQuery.data?.items?.length ?? 0}
                        </Descriptions.Item>
                    </Descriptions>

                    {activeDetailSystem?.description ? (
                        <div className="markdown-preview" style={{ padding: '16px', background: '#fafafa', borderRadius: 8 }}>
                            <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                {activeDetailSystem.description}
                            </ReactMarkdown>
                        </div>
                    ) : (
                        <ActionEmptyState
                            compact={true}
                            title={t('table.description')}
                            description={t('systems.description_placeholder')}
                            visual={<SystemsOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                        />
                    )}

                    <div className="workbench-detail-modal__table-scroll">
                        <Table<Service>
                            rowKey="id"
                            size="small"
                            loading={relatedServicesQuery.isLoading}
                            pagination={false}
                            scroll={{ x: 'max-content' }}
                            locale={{ emptyText: t('systems.related_services_empty', 'No services found for this system') }}
                            dataSource={relatedServicesQuery.data?.items ?? []}
                            columns={[
                                {
                                    title: t('table.name'),
                                    dataIndex: 'name',
                                    key: 'name',
                                    render: (name: string) => <Text strong>{name}</Text>,
                                },
                                {
                                    title: t('table.description'),
                                    dataIndex: 'description',
                                    key: 'description',
                                    render: (value?: string) => <Text type="secondary">{value || '—'}</Text>,
                                },
                                {
                                    title: t('services.next_instance_index', 'Next Instance Index'),
                                    dataIndex: 'next_instance_index',
                                    key: 'next_instance_index',
                                    width: 160,
                                    render: (value?: number) => <Text type="secondary">#{value ?? 0}</Text>,
                                },
                                {
                                    title: t('table.actions'),
                                    key: 'actions',
                                    width: 260,
                                    render: (_, service) => (
                                        <Space size="small">
                                            <Button
                                                type="link"
                                                size="small"
                                                onClick={() => {
                                                    setDetailOpen(false);
                                                    router.push(`/services?system_id=${service.system_id}&detail_service_id=${service.id}`);
                                                }}
                                            >
                                                {t('button.detail', 'Details')}
                                            </Button>
                                            <Button
                                                type="link"
                                                size="small"
                                                onClick={() => {
                                                    setDetailOpen(false);
                                                    router.push(`/vms?request=create&system_id=${service.system_id}&service_id=${service.id}`);
                                                }}
                                            >
                                                {t('button.request_vm')}
                                            </Button>
                                        </Space>
                                    ),
                                },
                            ]}
                        />
                    </div>
                </Space>
            </WorkbenchDetailModal>
        </div>
    );
}
