'use client';

import {
    Button,
    Card,
    Descriptions,
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
import React, { useMemo, useState } from 'react';
import { CloudOutlined, DeleteOutlined, EditOutlined, EyeOutlined, PlusOutlined, ReloadOutlined, UploadOutlined, FileTextOutlined, DesktopOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSanitize from 'rehype-sanitize';
import { useRouter, useSearchParams } from 'next/navigation';
import { useTranslation } from 'react-i18next';

import { PermissionGuard } from '@/components/auth/PermissionGuard';
import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import {
    RequestsOverviewGlyph,
    ServiceWorkspaceGlyph,
    VirtualMachinesOverviewGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { PageSearchToolbar, filterOptionByLabel } from '@/components/ui/PageSearchToolbar';
import { WorkbenchDetailModal } from '@/components/workbench/WorkbenchDetailModal';
import {
    buildDashboardSetupResumeHref,
    resolveNextSetupAction,
} from '@/features/setup-guide/flow';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import { useAutoOpenIntent } from '@/features/setup-guide/hooks/useAutoOpenIntent';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { approvalSummaryMeta, approvalSummaryTitle, formatApprovalRecordID } from '@/features/approval-shared/summary';
import { ALL_SYSTEMS_FILTER, useServicesManagementController } from '../hooks/useServicesManagementController';
import type { Ticket, Service, ServiceWorkspaceContext, VM } from '../types';

const { Text } = Typography;

export function ServicesManagementContent() {
    const { t } = useTranslation(['common', 'vm', 'approval']);
    const router = useRouter();
    const searchParams = useSearchParams();
    const initialSystemId = searchParams.get('system_id') || undefined;
    const detailServiceIdFromQuery = searchParams.get('detail_service_id') || undefined;
    const setupGuide = useSetupGuide();
    const services = useServicesManagementController({
        t,
        initialSystemId,
        onCreateSuccess: (_, context) => {
            if (!context.isFirstService) {
                return false;
            }
            const nextAction = resolveNextSetupAction(setupGuide, 'service');
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
    const [detailService, setDetailService] = useState<Service | null>(null);
    const [dismissedQueryDetailServiceId, setDismissedQueryDetailServiceId] = useState<string | null>(null);
    const [quickSearchDraft, setQuickSearchDraft] = useState(() => services.filters.search);
    const [systemFilterDraft, setSystemFilterDraft] = useState(() => services.filters.systemId);
    const [filtersOpen, setFiltersOpen] = useState(() => services.filters.systemId !== ALL_SYSTEMS_FILTER);

    useAutoOpenIntent('create-service', (params) => {
        services.openCreateModal(params.get('system_id') ?? undefined);
    });

    const deepLinkedService = useMemo(() => {
        if (!detailServiceIdFromQuery || detailServiceIdFromQuery === dismissedQueryDetailServiceId) {
            return null;
        }
        return (
            services.servicesData?.items?.find((service) => service.id === detailServiceIdFromQuery) ?? null
        );
    }, [detailServiceIdFromQuery, dismissedQueryDetailServiceId, services.servicesData?.items]);

    const activeDetailService = detailService ?? deepLinkedService;
    const detailModalOpen = detailOpen || Boolean(activeDetailService);

    const closeDetailModal = () => {
        setDetailOpen(false);
        setDetailService(null);
        if (detailServiceIdFromQuery) {
            setDismissedQueryDetailServiceId(detailServiceIdFromQuery);
            const url = new URL(window.location.href);
            url.searchParams.delete('detail_service_id');
            const nextURL = `${url.pathname}${url.searchParams.toString() === '' ? '' : `?${url.searchParams.toString()}`}${url.hash}`;
            window.history.replaceState(window.history.state, '', nextURL);
        }
    };

    const serviceContextQuery = useApiGet<ServiceWorkspaceContext>(
        ['service-workspace-context', activeDetailService?.system_id, activeDetailService?.id],
        () => api.GET('/systems/{system_id}/services/{service_id}/context', {
            params: {
                path: {
                    system_id: activeDetailService!.system_id,
                    service_id: activeDetailService!.id,
                },
            },
        }),
        { enabled: detailModalOpen && Boolean(activeDetailService?.id) }
    );

    const detailSystemName = serviceContextQuery.data?.service.system_name
        ?? activeDetailService?.system_name
        ?? services.systemsData?.items?.find((system) => system.id === activeDetailService?.system_id)?.name
        ?? '—';

    const serviceRelatedVMs = serviceContextQuery.data?.visible_vms ?? [];
    const serviceRelatedRequests = serviceContextQuery.data?.recent_requests ?? [];

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
            title: t('services.system_column'),
            dataIndex: 'system_name',
            key: 'system_name',
            width: 180,
            render: (_: string | undefined, record) => (
                record.system_id && record.system_name ? (
                    <Button
                        type="link"
                        size="small"
                        data-testid={`service-action-open-system-${record.id}`}
                        style={{ paddingInline: 0, justifyContent: 'flex-start' }}
                        onClick={() => router.push(`/systems?detail_system_id=${record.system_id}`)}
                    >
                        {record.system_name}
                    </Button>
                ) : (
                    <Text strong>{record.system_name || '—'}</Text>
                )
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
            title: t('services.next_instance_index'),
            dataIndex: 'next_instance_index',
            key: 'next_instance_index',
            width: 180,
            render: (idx: number) => <Tag color="blue">#{idx ?? 0}</Tag>,
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
            title: t('table.actions'),
            key: 'actions',
            width: 320,
            render: (_, record) => (
                <Space>
                    <Button
                        type="link"
                        size="small"
                        data-testid={`service-action-detail-${record.id}`}
                        icon={<EyeOutlined />}
                            onClick={() => {
                                setDetailService(record);
                                setDetailOpen(true);
                                setDismissedQueryDetailServiceId(null);
                            }}
                    >
                        {t('common:button.detail', { defaultValue: 'Details' })}
                    </Button>
                    <PermissionGuard permission="service:create">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`service-action-edit-${record.id}`}
                            icon={<EditOutlined />}
                            loading={services.updatePending && services.editingService?.id === record.id}
                            onClick={() => services.openEditModal(record)}
                        >
                            {t('common:button.edit')}
                        </Button>
                    </PermissionGuard>
                    <PermissionGuard permission="vm:create">
                        <Button
                            type="link"
                            size="small"
                            data-testid={`service-action-request-vm-${record.id}`}
                            icon={<DesktopOutlined />}
                            onClick={() => {
                                const params = new URLSearchParams({
                                    request: 'create',
                                    system_id: record.system_id,
                                    service_id: record.id,
                                });
                                router.push(`/vms?${params.toString()}`);
                            }}
                        >
                            {t('services.request_vm')}
                        </Button>
                    </PermissionGuard>
                    <PermissionGuard permission="service:delete">
                        <Popconfirm
                            title={t('message.confirm_delete')}
                            onConfirm={() => services.submitDelete(record.system_id, record.id)}
                            okText={t('common:button.confirm')}
                            cancelText={t('common:button.cancel')}
                        >
                            <Button
                                type="link"
                                size="small"
                                data-testid={`service-action-delete-${record.id}`}
                                danger
                                icon={<DeleteOutlined />}
                                loading={services.deletePending}
                            >
                                {t('common:button.delete')}
                            </Button>
                        </Popconfirm>
                    </PermissionGuard>
                </Space>
            ),
        },
    ];
    const serviceItems = useMemo(
        () => services.servicesData?.items ?? [],
        [services.servicesData?.items],
    );
    const applySearch = (searchValue = quickSearchDraft) => {
        services.applyFilters({
            search: searchValue,
            systemId: systemFilterDraft,
        });
    };

    const clearSearch = () => {
        setQuickSearchDraft('');
        setSystemFilterDraft(ALL_SYSTEMS_FILTER);
        setFiltersOpen(false);
        services.clearFilters();
    };

    return (
        <div className="services-page">
            {services.messageContextHolder}
            <PageHeader
                title={t('nav.services')}
                subtitle={t('services.subtitle')}
                actions={(
                    <Space>
                    <Button icon={<ReloadOutlined />} onClick={() => services.refetch()}>
                        {t('common:button.refresh')}
                    </Button>
                    <PermissionGuard permission="service:create">
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            data-testid="service-create-button"
                            onClick={() => {
                                services.openCreateModal();
                            }}
                        >
                            {t('common:button.create')}
                        </Button>
                    </PermissionGuard>
                    </Space>
                )}
            />

            {serviceItems.length === 0 && !services.isLoading && !services.hasActiveFilters && services.activeSystemId === ALL_SYSTEMS_FILTER ? (
                <SetupGuideCard variant="services" />
            ) : (
                <PageSurface className="services-page__table-surface" flush={true}>
                    <div style={{ padding: 16, paddingBottom: 0 }}>
                        <PageSearchToolbar
                            searchValue={services.filters.search}
                            searchDraftValue={quickSearchDraft}
                            onSearchDraftChange={setQuickSearchDraft}
                            onSearchChange={(value) => {
                                setQuickSearchDraft(value);
                                services.applyFilters({
                                    search: value,
                                    systemId: services.filters.systemId,
                                });
                            }}
                            searchPlaceholder={t('services.search_placeholder', 'Search services by name, system, description, or instance index')}
                            searchTestId="services-quick-search"
                            searchHelp={t('services.search_help', 'Press Enter or click Search. Quick search matches service names, descriptions, system names, and instance indexes.')}
                            advancedSearch={{
                                open: filtersOpen,
                                onToggle: () => setFiltersOpen((current) => !current),
                                openLabel: t('search.advanced', 'Advanced search'),
                                closeLabel: t('search.hide_advanced', 'Hide advanced search'),
                                title: t('search.advanced', 'Advanced search'),
                                content: (
                                    <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                        <Text type="secondary">
                                            {t('services.advanced_search_help', 'Select exact service filters here. Options support keyword matching, but the applied filter remains an exact value.')}
                                        </Text>
                                        <Space wrap size={[12, 12]} align="end">
                                        <Select
                                            data-testid="services-system-selector"
                                            style={{ minWidth: 240 }}
                                            placeholder={t('services.select_system')}
                                            showSearch
                                            filterOption={filterOptionByLabel}
                                            optionFilterProp="label"
                                            value={systemFilterDraft || undefined}
                                            onChange={(value) => {
                                                setSystemFilterDraft((value as string | undefined) ?? ALL_SYSTEMS_FILTER);
                                            }}
                                            options={[
                                                {
                                                    label: t('services.all_systems'),
                                                    value: ALL_SYSTEMS_FILTER,
                                                },
                                                ...(services.systemsData?.items?.map((system) => ({
                                                    label: system.name,
                                                    value: system.id,
                                                })) ?? []),
                                            ]}
                                        />
                                        <Button
                                            type="primary"
                                            data-testid="services-advanced-search-submit"
                                            onClick={() => applySearch()}
                                        >
                                            {t('common:button.search')}
                                        </Button>
                                        </Space>
                                    </Space>
                                ),
                            }}
                            hasActiveFilters={services.hasActiveFilters}
                            onClear={clearSearch}
                            clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                        />
                    </div>
                    <Table<Service>
                        columns={columns}
                        dataSource={serviceItems}
                        rowKey="id"
                        loading={services.isLoading}
                        scroll={{ x: 'max-content' }}
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
                        locale={{
                            emptyText: (
                                <ActionEmptyState
                                    compact={true}
                                    title={t('services.empty_filtered_title', 'No services match the current search')}
                                    description={t('services.empty_filtered_description', 'Try a broader search or clear the current filters.')}
                                    visual={<ServiceWorkspaceGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                />
                            ),
                        }}
                    />
                </PageSurface>
            )}

            <Modal
                title={t('services.modal.create_title')}
                open={services.createOpen}
                onOk={() => {
                    void services.submitCreate();
                }}
                onCancel={services.closeCreateModal}
                confirmLoading={services.createPending}
                data-testid="service-create-modal"
            >
                <Form form={services.form} layout="vertical" name="create-service">
                    <Form.Item
                        name="system_id"
                        label={t('services.form.system_label')}
                        rules={[{ required: true, message: t('services.validation.system_required') }]}
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
            {detailModalOpen && activeDetailService ? (
            <WorkbenchDetailModal
                title={activeDetailService.name}
                open={detailModalOpen}
                onCancel={closeDetailModal}
                footer={[
                    <Button
                        key="request-vm"
                        type="primary"
                        onClick={() => {
                            const params = new URLSearchParams({
                                request: 'create',
                                system_id: activeDetailService.system_id,
                                service_id: activeDetailService.id,
                            });
                            closeDetailModal();
                            router.push(`/vms?${params.toString()}`);
                        }}
                    >
                        {t('services.request_vm')}
                    </Button>,
                    <Button
                        key="open-requests"
                        onClick={() => {
                            closeDetailModal();
                            router.push('/tickets?tab=history');
                        }}
                    >
                        {t('services.open_my_requests')}
                    </Button>,
                    <Button key="close" onClick={closeDetailModal}>
                        {t('common:button.close', { defaultValue: 'Close' })}
                    </Button>
                ]}
                width="min(1120px, calc(100vw - 16px))"
                contentMinWidth={1080}
            >
                <Space direction="vertical" size={16} className="workbench-detail-modal__stack">
                    <Card size="small" title={t('services.context_title')}>
                        <Descriptions
                            size="small"
                            column={{ xs: 1, sm: 1, md: 1, lg: 2, xl: 2, xxl: 2 }}
                        >
                            <Descriptions.Item label={t('services.context_system')}>
                                {activeDetailService?.system_id && detailSystemName !== '—' ? (
                                    <Button
                                        type="link"
                                        size="small"
                                        style={{ paddingInline: 0, justifyContent: 'flex-start' }}
                                        onClick={() => router.push(`/systems?detail_system_id=${activeDetailService.system_id}`)}
                                    >
                                        {detailSystemName}
                                    </Button>
                                ) : (
                                    detailSystemName
                                )}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('services.context_next_index')}>
                                <Tag color="blue">{serviceContextQuery.data?.service.next_instance_index ?? detailService?.next_instance_index ?? 0}</Tag>
                            </Descriptions.Item>
                            <Descriptions.Item label={t('table.created_at')}>
                                <LocalDateTimeText value={serviceContextQuery.data?.service.created_at ?? detailService?.created_at} />
                            </Descriptions.Item>
                            <Descriptions.Item label={t('services.context_summary')}>
                                {t('services.context_summary_value', {
                                    vmCount: serviceContextQuery.data?.summary.visible_vm_count ?? 0,
                                    requestCount: serviceContextQuery.data?.summary.recent_request_count ?? 0,
                                })}
                            </Descriptions.Item>
                        </Descriptions>
                    </Card>

                    <Card size="small" title={t('table.description')}>
                        {(serviceContextQuery.data?.service.description ?? detailService?.description) ? (
                            <div className="markdown-preview" style={{ padding: '8px 0' }}>
                                <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                    {serviceContextQuery.data?.service.description ?? detailService?.description ?? ''}
                                </ReactMarkdown>
                            </div>
                        ) : (
                            <ActionEmptyState
                                compact={true}
                                title={t('table.description')}
                                description={t('services.empty_description')}
                                visual={<ServiceWorkspaceGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                            />
                        )}
                    </Card>

                    <Card
                        size="small"
                        title={t('services.related_vms_title')}
                        extra={(
                            <Button
                                size="small"
                                onClick={() => {
                                    if (!activeDetailService) {
                                        return;
                                    }
                                    router.push(
                                        `/vms?system_id=${activeDetailService.system_id}&service_id=${activeDetailService.id}`,
                                    );
                                }}
                            >
                                {t('services.open_vm_workspace')}
                            </Button>
                        )}
                    >
                        <div className="workbench-detail-modal__table-scroll">
                            <Table<VM>
                                rowKey="id"
                                size="small"
                                loading={serviceContextQuery.isLoading}
                                pagination={false}
                                scroll={{ x: 'max-content' }}
                                dataSource={serviceRelatedVMs}
                                locale={{
                                    emptyText: (
                                        <ActionEmptyState
                                            compact={true}
                                            title={t('services.related_vms_title')}
                                            description={t('services.related_vms_empty')}
                                            visual={<VirtualMachinesOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                        />
                                    ),
                                }}
                                columns={[
                                    {
                                        title: t('table.name'),
                                        dataIndex: 'name',
                                        key: 'name',
                                        render: (name: string) => <Text strong>{name}</Text>,
                                    },
                                    {
                                        title: t('field.namespace', { ns: 'vm' }),
                                        dataIndex: 'namespace',
                                        key: 'namespace',
                                    },
                                    {
                                        title: t('table.status'),
                                        dataIndex: 'status',
                                        key: 'status',
                                        render: (status: string) => <Tag>{status}</Tag>,
                                    },
                                    {
                                        title: t('table.actions'),
                                        key: 'actions',
                                        render: (_, record) => (
                                            <Button
                                                type="link"
                                                size="small"
                                                onClick={() => router.push(`/vms/${record.id}`)}
                                            >
                                                {t('services.open_vm_detail')}
                                            </Button>
                                        ),
                                    },
                                ]}
                            />
                        </div>
                    </Card>

                    <Card
                        size="small"
                        title={t('services.related_requests_title')}
                        extra={(
                            <Button size="small" onClick={() => router.push('/tickets?tab=history')}>
                                {t('services.open_my_requests')}
                            </Button>
                        )}
                    >
                        <div className="workbench-detail-modal__table-scroll">
                            <Table<Ticket>
                                rowKey="id"
                                size="small"
                                loading={serviceContextQuery.isLoading}
                                pagination={false}
                                scroll={{ x: 'max-content' }}
                                dataSource={serviceRelatedRequests}
                                locale={{
                                    emptyText: (
                                        <ActionEmptyState
                                            compact={true}
                                            title={t('services.related_requests_title')}
                                            description={t('services.related_requests_empty')}
                                            visual={<RequestsOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                        />
                                    ),
                                }}
                                columns={[
                                    {
                                        title: t('request_summary', { ns: 'approval' }),
                                        key: 'request_summary',
                                        render: (_, record) => {
                                            const summaryMeta = approvalSummaryMeta(record, t);
                                            return (
                                                <Space direction="vertical" size={0}>
                                                    <Space size={8}>
                                                        <FileTextOutlined style={{ color: '#1677ff' }} />
                                                        <Text strong>{approvalSummaryTitle(record, t)}</Text>
                                                    </Space>
                                                    {summaryMeta.length > 0 && (
                                                        <Text type="secondary" style={{ fontSize: 12 }}>
                                                            {summaryMeta.join(' · ')}
                                                        </Text>
                                                    )}
                                                    <Text copyable={{ text: record.id }} type="secondary" style={{ fontSize: 12 }}>
                                                        {t('ticket_id', { ns: 'approval' })}: {formatApprovalRecordID(record.id)}
                                                    </Text>
                                                </Space>
                                            );
                                        },
                                    },
                                    {
                                        title: t('operation_type', { ns: 'approval' }),
                                        dataIndex: 'operation_type',
                                        key: 'operation_type',
                                        render: (value: string) => <Tag>{value || '—'}</Tag>,
                                    },
                                    {
                                        title: t('table.status'),
                                        dataIndex: 'status',
                                        key: 'status',
                                        render: (status: string) => <Tag>{status}</Tag>,
                                    },
                                    {
                                        title: t('table.created_at'),
                                        dataIndex: 'created_at',
                                        key: 'created_at',
                                        render: (date: string) => <LocalDateTimeText value={date} />,
                                    },
                                ]}
                            />
                        </div>
                    </Card>
                </Space>
            </WorkbenchDetailModal>
            ) : null}
        </div>
    );
}
