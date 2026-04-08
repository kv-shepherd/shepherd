'use client';

import React, { useMemo, useState } from 'react';
import {
    Button,
    Descriptions,
    Form,
    Input,
    Modal,
    Select,
    Space,
    Table,
    Tooltip,
    Typography,
    Upload,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
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
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
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

const filterOptionByLabel = (input: string, option?: { label?: unknown }) => {
    const label = typeof option?.label === 'string' ? option.label : '';
    return label.toLowerCase().includes(input.trim().toLowerCase());
};

const SystemEntityIcon = () => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" width="1.3em" height="1.3em">
        <rect x="3" y="4" width="18" height="16" rx="2" ry="2" />
        <line x1="3" y1="10" x2="21" y2="10" />
        <line x1="3" y1="16" x2="21" y2="16" />
        <line x1="7" y1="7" x2="7.01" y2="7" strokeWidth="2.5" />
        <line x1="11" y1="7" x2="11.01" y2="7" strokeWidth="2.5" />
    </svg>
);

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
        <Space size={[6, 6]} wrap>
            {previewItems.map((service) => (
                <div
                    key={service.id}
                    className="system-service-chip hover-scale transition-all"
                    data-testid={`system-service-link-${service.id}`}
                    style={{
                        padding: '2px 8px',
                        background: '#f8fafc',
                        border: '1px solid #e2e8f0',
                        borderRadius: 12,
                        fontSize: 12,
                        color: '#475569',
                        cursor: 'pointer',
                        display: 'flex',
                        alignItems: 'center',
                        gap: 6
                    }}
                    onClick={() => onOpenService(service)}
                >
                    <div style={{ width: 6, height: 6, borderRadius: '50%', background: '#10b981' }} />
                    <span style={{ fontWeight: 500 }}>{service.name}</span>
                </div>
            ))}
            {remaining > 0 ? (
                <div style={{ fontSize: 12, color: '#64748b', padding: '2px 4px', fontWeight: 500 }}>
                    +{remaining} more
                </div>
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
    const [quickSearchDraft, setQuickSearchDraft] = useState(() => systems.filters.search);
    const [createdByDraft, setCreatedByDraft] = useState(() => systems.filters.createdBy);
    const [serviceIdDraft, setServiceIdDraft] = useState(() => systems.filters.serviceId);
    const [memberIdDraft, setMemberIdDraft] = useState(() => systems.filters.memberId);
    const [filtersOpen, setFiltersOpen] = useState(
        () => systems.filters.createdBy !== ''
            || systems.filters.serviceId !== ''
            || systems.filters.memberId !== '',
    );
    const detailSystemIdFromQuery = searchParams.get('detail_system_id') || undefined;

    const activeDetailSystem = useMemo(() => {
        if (!detailSystemIdFromQuery || detailSystemIdFromQuery === dismissedQueryDetailSystemId) {
            return detailSystem;
        }
        return systems.data?.items?.find((system) => system.id === detailSystemIdFromQuery) ?? detailSystem;
    }, [detailSystem, detailSystemIdFromQuery, dismissedQueryDetailSystemId, systems.data?.items]);

    const activeDetailDisplayName = activeDetailSystem?.created_by_display_name?.trim();
    const activeDetailUsername = activeDetailSystem?.created_by_username?.trim();
    const activeDetailCreatorLabel =
        activeDetailDisplayName && activeDetailUsername && activeDetailDisplayName !== activeDetailUsername
            ? `${activeDetailDisplayName} · ${activeDetailUsername}`
            : activeDetailDisplayName || activeDetailUsername || activeDetailSystem?.created_by?.trim() || '—';

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
            render: (name: string, record) => (
                <Space size={12}>
                    <div style={{
                        width: 36,
                        height: 36,
                        borderRadius: 10,
                        background: 'linear-gradient(135deg, rgba(94, 106, 210, 0.12) 0%, rgba(94, 106, 210, 0.04) 100%)',
                        color: '#5E6AD2',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        boxShadow: 'inset 0 1px 2px rgba(255, 255, 255, 0.6), 0 1px 2px rgba(0, 0, 0, 0.02)',
                        border: '1px solid rgba(94, 106, 210, 0.1)',
                    }}>
                        <SystemEntityIcon />
                    </div>
                    <div style={{ display: 'flex', flexDirection: 'column' }}>
                        <Text strong style={{ fontSize: 14, color: '#1e293b' }}>{name}</Text>
                        <Text type="secondary" style={{ fontSize: 12 }}>ID: {record.id.slice(0, 8)}</Text>
                    </div>
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
            key: 'created_by',
            width: 200,
            render: (_, record) => {
                const displayName = record.created_by_display_name?.trim();
                const username = record.created_by_username?.trim();
                const fallback = record.created_by?.trim() || '—';
                return (
                    <Space direction="vertical" size={0} className="workbench-table-stack">
                        <Text strong>{displayName || username || fallback}</Text>
                        {displayName && username && displayName !== username ? (
                            <Text type="secondary" className="workbench-table-note">
                                {username}
                            </Text>
                        ) : displayName || username ? null : null}
                    </Space>
                );
            },
        },
        {
            title: t('table.created_at'),
            dataIndex: 'created_at',
            key: 'created_at',
            width: 160,
            render: (date: string) => (
                <Space direction="vertical" size={2}>
                    <Text style={{ color: '#334155' }}><LocalDateTimeText value={date} /></Text>
                </Space>
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
                <Space size={4}>
                    <Tooltip title={t('common:button.detail', { defaultValue: 'Details' })}>
                        <Button
                            type="text"
                            size="small"
                            data-testid={`system-action-detail-${record.id}`}
                            icon={<EyeOutlined />}
                            onClick={() => {
                                setDetailSystem(record);
                                setDetailOpen(true);
                                setDismissedQueryDetailSystemId(null);
                            }}
                        />
                    </Tooltip>
                    <PermissionGuard permission="rbac:manage">
                        <Tooltip title={t('common:button.manage_members')}>
                            <Button
                                type="text"
                                size="small"
                                data-testid={`system-action-members-${record.id}`}
                                icon={<TeamOutlined />}
                                onClick={() => systems.openMembersModal(record)}
                            />
                        </Tooltip>
                    </PermissionGuard>
                    <PermissionGuard permission="system:write">
                        <Tooltip title={t('common:button.edit')}>
                            <Button
                                type="text"
                                size="small"
                                data-testid={`system-action-edit-${record.id}`}
                                icon={<EditOutlined />}
                                loading={systems.updatePending && systems.editingSystem?.id === record.id}
                                onClick={() => systems.openEditModal(record)}
                            />
                        </Tooltip>
                    </PermissionGuard>
                    <PermissionGuard permission="system:delete">
                        <Tooltip title={t('common:button.delete')}>
                            <Button
                                type="text"
                                size="small"
                                data-testid={`system-action-delete-${record.id}`}
                                danger
                                icon={<DeleteOutlined />}
                                loading={systems.deletePending && systems.deletingSystem?.id === record.id}
                                onClick={() => systems.openDeleteModal(record)}
                            />
                        </Tooltip>
                    </PermissionGuard>
                </Space>
            ),
        },
    ];
    const systemItems = useMemo(() => systems.data?.items ?? [], [systems.data?.items]);
    const creatorOptions = useMemo(
        () =>
            (systems.systemFilterOptions?.creators ?? []).map((option) => ({
                value: option.value,
                label: option.label,
            })),
        [systems.systemFilterOptions?.creators],
    );
    const serviceOptions = useMemo(
        () =>
            (systems.systemFilterOptions?.services ?? []).map((option) => ({
                value: option.value,
                label: option.group ? `${option.group} · ${option.label}` : option.label,
            })),
        [systems.systemFilterOptions?.services],
    );
    const memberOptions = useMemo(
        () =>
            (systems.systemFilterOptions?.members ?? []).map((option) => ({
                value: option.value,
                label: option.label,
            })),
        [systems.systemFilterOptions?.members],
    );
    const applySearch = (searchValue = quickSearchDraft) => {
        systems.applyFilters({
            search: searchValue,
            createdBy: createdByDraft,
            serviceId: serviceIdDraft,
            memberId: memberIdDraft,
        });
    };

    const clearSearch = () => {
        setQuickSearchDraft('');
        setCreatedByDraft('');
        setServiceIdDraft('');
        setMemberIdDraft('');
        setFiltersOpen(false);
        systems.clearFilters();
    };

    return (
        <div className="systems-page">
            {systems.messageContextHolder}
            <PageHeader
                title={t('nav.systems')}
                subtitle={t('systems.subtitle')}
                actions={(
                    <Space>
                        <Button icon={<ReloadOutlined />} onClick={() => systems.refetch()}>
                            {t('common:button.refresh')}
                        </Button>
                        <PermissionGuard permission="system:write">
                            <Button
                                type="primary"
                                icon={<PlusOutlined />}
                                data-testid="system-create-button"
                                onClick={systems.openCreateModal}
                            >
                                {t('common:button.create')}
                            </Button>
                        </PermissionGuard>
                    </Space>
                )}
            />

            {systemItems.length === 0 && !systems.isLoading && !systems.hasActiveFilters ? (
                <SetupGuideCard variant="systems" />
            ) : (
                <PageSurface className="systems-page__table-surface" flush={true}>
                    <div style={{ padding: 16, paddingBottom: 0 }}>
                        <PageSearchToolbar
                            searchValue={systems.filters.search}
                            searchDraftValue={quickSearchDraft}
                            onSearchDraftChange={setQuickSearchDraft}
                            onSearchChange={(value) => {
                                setQuickSearchDraft(value);
                                systems.applyFilters({
                                    search: value,
                                    createdBy: systems.filters.createdBy,
                                    serviceId: systems.filters.serviceId,
                                    memberId: systems.filters.memberId,
                                });
                            }}
                            searchPlaceholder={t('systems.search_placeholder', 'Search systems, services, or members')}
                            searchTestId="systems-quick-search"
                            searchHelp={t('systems.search_help', 'Press Enter or click Search. Quick search matches system names, descriptions, creators, related services, members, and pasted IDs.')}
                            advancedSearch={{
                                open: filtersOpen,
                                onToggle: () => setFiltersOpen((open) => !open),
                                openLabel: t('search.advanced', { defaultValue: 'Advanced search' }),
                                closeLabel: t('search.hide_advanced', { defaultValue: 'Hide advanced search' }),
                                title: t('search.advanced', { defaultValue: 'Advanced search' }),
                                toggleTestId: 'systems-search-filters-toggle',
                                content: (
                                    <Space direction="vertical" size={12} style={{ width: '100%' }}>
                                        <Text type="secondary">
                                            {t('systems.search.advanced_help', 'Select exact filters here. Options support keyword matching, but the applied filter remains an exact match.')}
                                        </Text>
                                        <Space wrap size={[12, 12]} align="end">
                                            <Select
                                                allowClear
                                                showSearch
                                                filterOption={filterOptionByLabel}
                                                optionFilterProp="label"
                                                style={{ width: 220 }}
                                                data-testid="systems-filter-created-by"
                                                placeholder={t('systems.search.created_by', 'Created by')}
                                                value={createdByDraft || undefined}
                                                options={creatorOptions}
                                                loading={systems.systemFilterOptionsLoading}
                                                onChange={(value) => setCreatedByDraft(value ?? '')}
                                            />
                                            <Select
                                                allowClear
                                                showSearch
                                                filterOption={filterOptionByLabel}
                                                optionFilterProp="label"
                                                style={{ width: 260 }}
                                                data-testid="systems-filter-service"
                                                placeholder={t('systems.search.service', 'Related service')}
                                                value={serviceIdDraft || undefined}
                                                options={serviceOptions}
                                                loading={systems.systemFilterOptionsLoading}
                                                onChange={(value) => setServiceIdDraft(value ?? '')}
                                            />
                                            <Select
                                                allowClear
                                                showSearch
                                                filterOption={filterOptionByLabel}
                                                optionFilterProp="label"
                                                style={{ width: 280 }}
                                                data-testid="systems-filter-member"
                                                placeholder={t('systems.search.member', 'Member')}
                                                value={memberIdDraft || undefined}
                                                options={memberOptions}
                                                loading={systems.systemFilterOptionsLoading}
                                                onChange={(value) => setMemberIdDraft(value ?? '')}
                                            />
                                            <Button
                                                type="primary"
                                                data-testid="systems-advanced-search-submit"
                                                onClick={() => applySearch()}
                                            >
                                                {t('common:button.search')}
                                            </Button>
                                        </Space>
                                    </Space>
                                ),
                            }}
                            hasActiveFilters={systems.hasActiveFilters}
                            onClear={clearSearch}
                            clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
                        />
                    </div>
                    <Table<System>
                        columns={columns}
                        dataSource={systemItems}
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
                        locale={{
                            emptyText: (
                                <ActionEmptyState
                                    compact={true}
                                    title={t('systems.empty_filtered_title', 'No systems match the current search')}
                                    description={t('systems.empty_filtered_description', 'Try a broader search or clear the current query.')}
                                    visual={<SystemsOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                />
                            ),
                        }}
                    />
                </PageSurface>
            )}

            {systems.createOpen ? (
            <Modal
                title={t('systems.modal.create_title')}
                open={systems.createOpen}
                onOk={() => {
                    void systems.submitCreate();
                }}
                onCancel={systems.closeCreateModal}
                confirmLoading={systems.createPending}
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
            ) : null}

            {systems.editOpen ? (
            <Modal
                title={t('systems.modal.edit_title')}
                open={systems.editOpen}
                onOk={() => {
                    void systems.submitEdit();
                }}
                onCancel={systems.closeEditModal}
                confirmLoading={systems.updatePending}
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
            ) : null}

            {systems.deleteOpen ? (
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
                okText={t('common:button.delete')}
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
            ) : null}

            <SystemMembersModal
                open={systems.membersOpen}
                onCancel={systems.closeMembersModal}
                systemId={systems.membersSystem?.id ?? null}
                systemName={systems.membersSystem?.name}
            />

            {detailModalOpen && activeDetailSystem ? (
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
                        {t('common:button.close', { defaultValue: 'Close' })}
                    </Button>
                ]}
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
                            {activeDetailCreatorLabel}
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
                                                {t('common:button.detail', { defaultValue: 'Details' })}
                                            </Button>
                                            <Button
                                                type="link"
                                                size="small"
                                                onClick={() => {
                                                    setDetailOpen(false);
                                                    router.push(`/vms?request=create&system_id=${service.system_id}&service_id=${service.id}`);
                                                }}
                                            >
                                                {t('common:button.request_vm')}
                                            </Button>
                                        </Space>
                                    ),
                                },
                            ]}
                        />
                    </div>
                </Space>
            </WorkbenchDetailModal>
            ) : null}
        </div>
    );
}
