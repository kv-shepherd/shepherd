'use client';

import React, { useMemo, useState } from 'react';
import {
    Button,
    Card,
    Form,
    Input,
    Modal,
    Select,
    Space,
    Table,
    Tag,
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
    InfoCircleOutlined,
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
import { SystemsIcon } from '@/components/layouts/MenuIcons';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import { WorkbenchDetailModal } from '@/components/workbench/WorkbenchDetailModal';
import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import { createRfc1035NameRule } from '@/lib/validation/rfc1035Name';
import {
    buildDashboardSetupResumeHref,
    resolveNextSetupAction,
} from '@/features/setup-guide/flow';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import { useAutoOpenIntent } from '@/features/setup-guide/hooks/useAutoOpenIntent';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useSystemsManagementController } from '../hooks/useSystemsManagementController';
import type { System, SystemMemberList } from '../types';
import { SystemMembersModal } from './SystemMembersModal';
import type { components } from '@/types/api.gen';

const { Text, Paragraph } = Typography;
type Service = components['schemas']['Service'];
type ServiceList = components['schemas']['ServiceList'];
type SystemMember = components['schemas']['SystemMember'];

const filterOptionByLabel = (input: string, option?: { label?: unknown }) => {
    const label = typeof option?.label === 'string' ? option.label : '';
    return label.toLowerCase().includes(input.trim().toLowerCase());
};

function truncateUtf8(input: string, maxBytes: number): string {
    const encoder = new TextEncoder();
    if (encoder.encode(input).length <= maxBytes) {
        return input;
    }

    let truncated = '';
    for (const char of input) {
        const next = `${truncated}${char}`;
        if (encoder.encode(next).length > maxBytes) {
            break;
        }
        truncated = next;
    }

    return `${truncated.trimEnd()}…`;
}

function buildDescriptionPreview(input: string, maxBytes: number): string {
    const normalized = input
        .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
        .replace(/^[#>*-\s]+/gm, '')
        .replace(/[`*_~]/g, '')
        .replace(/\s+/g, ' ')
        .trim();

    return truncateUtf8(normalized, maxBytes);
}

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

    const previewItems = items.slice(0, 2);
    const remaining = items.length - previewItems.length;

    return (
        <Space size={[6, 6]} wrap>
            <Tag color="blue" className="system-service-chip__count">
                {t('systems.detail_services_badge', { count: items.length })}
            </Tag>
            {previewItems.map((service) => (
                <button
                    type="button"
                    key={service.id}
                    className="system-service-chip"
                    data-testid={`system-service-link-${service.id}`}
                    onClick={() => onOpenService(service)}
                >
                    <span className="system-service-chip__dot" />
                    <span className="system-service-chip__label">{service.name}</span>
                </button>
            ))}
            {remaining > 0 ? (
                <div className="system-service-chip__more">
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
    const systemNameRules = useMemo(() => [
        createRfc1035NameRule(
            {
                required: t('systems.validation.name_required'),
                max: t('systems.validation.name_max'),
                format: t('systems.validation.name_format'),
            },
            { maxLength: 15 },
        ),
    ], [t]);
    const [detailOpen, setDetailOpen] = useState(false);
    const [detailSystem, setDetailSystem] = useState<System | null>(null);
    const [dismissedQueryDetailSystemId, setDismissedQueryDetailSystemId] = useState<string | null>(null);
    const [membersDirectoryOpen, setMembersDirectoryOpen] = useState(false);
    const [membersDirectorySearchDraft, setMembersDirectorySearchDraft] = useState('');
    const [membersDirectorySearch, setMembersDirectorySearch] = useState('');
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
        setMembersDirectoryOpen(false);
        setMembersDirectorySearchDraft('');
        setMembersDirectorySearch('');
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
    const relatedMembersQuery = useApiGet<SystemMemberList>(
        ['system-related-members', activeDetailSystem?.id],
        () => api.GET('/systems/{system_id}/members', {
            params: {
                path: { system_id: activeDetailSystem!.id },
            },
        }),
        { enabled: detailModalOpen && Boolean(activeDetailSystem?.id) },
    );
    const detailServiceCount = relatedServicesQuery.data?.pagination?.total ?? relatedServicesQuery.data?.items?.length ?? 0;
    const detailMembers = relatedMembersQuery.data?.items ?? [];
    const detailMemberCount = relatedMembersQuery.data?.pagination?.total ?? detailMembers.length;
    const detailMemberRoleSummary = detailMembers.reduce(
        (summary, member) => {
            summary[member.role] += 1;
            return summary;
        },
        { owner: 0, admin: 0, member: 0, viewer: 0 } as Record<SystemMember['role'], number>,
    );
    const detailMemberPreview = detailMembers.slice(0, 6);
    const filteredDetailMembers = (() => {
        const keyword = membersDirectorySearch.trim().toLowerCase();
        if (!keyword) {
            return detailMembers;
        }
        return detailMembers.filter((member) => {
            const values = [
                member.display_name,
                member.email,
                member.username,
                member.user_id,
                t(`role.${member.role}`, { defaultValue: member.role }),
            ];
            return values.some((value) => value?.toLowerCase().includes(keyword));
        });
    })();

    useAutoOpenIntent('create-system', () => {
        systems.openCreateModal();
    });

    const columns: ColumnsType<System> = [
        {
            title: t('table.name'),
            dataIndex: 'name',
            key: 'name',
            render: (name: string) => (
                <div className="systems-table-name">
                    <div className="systems-table-name__icon">
                        <SystemsIcon />
                    </div>
                    <Text strong className="systems-table-name__text">
                        {name}
                    </Text>
                </div>
            ),
        },
        {
            title: t('table.description'),
            dataIndex: 'description',
            key: 'description',
            width: 220,
            render: (desc: string, record: System) => {
                const normalizedDescription = desc?.trim();
                if (!normalizedDescription) {
                    return <Text type="secondary">—</Text>;
                }
                const preview = buildDescriptionPreview(normalizedDescription, 36);
                return (
                    <div className="workbench-description-preview">
                        <Text type="secondary" className="workbench-table-description workbench-table-description--preview">
                            {preview}
                        </Text>
                        <Tooltip title={t('common:description.preview_tooltip')}>
                            <Button
                                type="text"
                                size="small"
                                className="workbench-icon-button"
                                icon={<InfoCircleOutlined />}
                                onClick={() => {
                                    setDetailSystem(record);
                                    setDetailOpen(true);
                                    setDismissedQueryDetailSystemId(null);
                                }}
                            />
                        </Tooltip>
                    </div>
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
                    <Text className="workbench-table-date"><LocalDateTimeText value={date} /></Text>
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
            width: 220,
            render: (_, record) => (
                <Space wrap size={[4, 4]} className="workbench-row-actions">
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
                        <div className="systems-table-workspace">
                            <div className="systems-table-workspace__metrics">
                                <div className="systems-table-workspace__metric">
                                    <span className="systems-table-workspace__metric-label">{t('nav.systems')}</span>
                                    <span className="systems-table-workspace__metric-value">{systems.data?.pagination?.total ?? systemItems.length}</span>
                                </div>
                                <div className="systems-table-workspace__metric">
                                    <span className="systems-table-workspace__metric-label">{t('systems.summary.related_services_title', 'Services')}</span>
                                    <span className="systems-table-workspace__metric-value systems-table-workspace__metric-value--muted">{serviceOptions.length}</span>
                                </div>
                                <div className="systems-table-workspace__metric">
                                    <span className="systems-table-workspace__metric-label">{t('systems.summary.members_title', 'Members')}</span>
                                    <span className="systems-table-workspace__metric-value systems-table-workspace__metric-value--muted">{memberOptions.length}</span>
                                </div>
                            </div>
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
                                                {t('common:search.exact_match_help')}
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

            <Modal
                title={t('systems.modal.create_title')}
                open={systems.createOpen}
                forceRender={true}
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
                        rules={systemNameRules}
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
                                    {createPreviewMode
                                        ? t('common:button.edit', { defaultValue: 'Edit' })
                                        : t('common:button.preview', { defaultValue: 'Preview' })}
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
                                        {t('common:button.upload_markdown', { defaultValue: 'Upload .md' })}
                                    </Button>
                                </Upload>
                            </Space>
                        }
                    >
                        <Form.Item noStyle shouldUpdate>
                            {(form) => (
                                <div className="markdown-preview markdown-editor-preview" hidden={!createPreviewMode}>
                                    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                        {form.getFieldValue('description') || t('common:markdown.empty', { defaultValue: 'No content provided' })}
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
                forceRender={true}
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
                                    {editPreviewMode
                                        ? t('common:button.edit', { defaultValue: 'Edit' })
                                        : t('common:button.preview', { defaultValue: 'Preview' })}
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
                                        {t('common:button.upload_markdown', { defaultValue: 'Upload .md' })}
                                    </Button>
                                </Upload>
                            </Space>
                        }
                    >
                        <Form.Item noStyle shouldUpdate>
                            {(form) => (
                                <div className="markdown-preview markdown-editor-preview" hidden={!editPreviewMode}>
                                    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                        {form.getFieldValue('description') || t('common:markdown.empty', { defaultValue: 'No content provided' })}
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
                footer={(
                    <Space>
                        <Button
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
                        </Button>
                        <Button onClick={closeDetailModal}>
                            {t('common:button.close', { defaultValue: 'Close' })}
                        </Button>
                    </Space>
                )}
                width="min(1120px, calc(100vw - 16px))"
                contentMinWidth={1040}
            >
                <Space direction="vertical" size={16} className="workbench-detail-modal__stack">
                    <Card size="small" className="workbench-detail-hero">
                        <Space direction="vertical" size={12} style={{ width: '100%' }}>
                            <Space size={[8, 8]} wrap>
                                <Tag color="blue">{t('nav.systems')}</Tag>
                                <Tag>{t('systems.detail_services_badge', { count: detailServiceCount, defaultValue: '{{count}} services' })}</Tag>
                                <Tag>{t('systems.detail_members_badge', { count: detailMemberCount, defaultValue: '{{count}} members' })}</Tag>
                            </Space>
                            <div className="workbench-detail-hero__grid">
                                <div className="workbench-compact-grid__item">
                                    <Text className="workbench-compact-grid__label">{t('table.created_by')}</Text>
                                    <Text strong className="workbench-compact-grid__value">{activeDetailCreatorLabel}</Text>
                                </div>
                                <div className="workbench-compact-grid__item">
                                    <Text className="workbench-compact-grid__label">{t('table.created_at')}</Text>
                                    <Text className="workbench-compact-grid__value">
                                        {activeDetailSystem.created_at ? <LocalDateTimeText value={activeDetailSystem.created_at} /> : '—'}
                                    </Text>
                                </div>
                                <div className="workbench-compact-grid__item">
                                    <Text className="workbench-compact-grid__label">{t('systems.related_services_column', 'Services')}</Text>
                                    <Text strong className="workbench-compact-grid__value">{detailServiceCount}</Text>
                                </div>
                                <div className="workbench-compact-grid__item">
                                    <Text className="workbench-compact-grid__label">{t('members.current_members_title', { defaultValue: 'Current Members' })}</Text>
                                    <Text strong className="workbench-compact-grid__value">{detailMemberCount}</Text>
                                </div>
                            </div>
                        </Space>
                    </Card>

                    <div className="workbench-detail-modal__grid">
                        <Card
                            size="small"
                            title={t('table.description')}
                            className="workbench-detail-section-card workbench-detail-section-card--primary"
                        >
                            {activeDetailSystem?.description ? (
                                <div className="markdown-preview">
                                    <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
                                        {activeDetailSystem.description}
                                    </ReactMarkdown>
                                </div>
                            ) : (
                                <ActionEmptyState
                                    compact={true}
                                    title={t('table.description')}
                                    description={t('systems.detail_description_empty', { defaultValue: 'No description is available for this system yet.' })}
                                    visual={<SystemsOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                />
                            )}
                        </Card>

                        <Card
                            size="small"
                            title={t('members.current_members_title', { defaultValue: 'Current Members' })}
                            extra={(
                                <Space size={6} wrap>
                                    {detailMemberRoleSummary.owner > 0 ? <Tag key="owner">{t('role.owner')}: {detailMemberRoleSummary.owner}</Tag> : null}
                                    {detailMemberRoleSummary.admin > 0 ? <Tag key="admin">{t('role.admin')}: {detailMemberRoleSummary.admin}</Tag> : null}
                                    {detailMemberRoleSummary.member > 0 ? <Tag key="member">{t('role.member')}: {detailMemberRoleSummary.member}</Tag> : null}
                                    {detailMemberRoleSummary.viewer > 0 ? <Tag key="viewer">{t('role.viewer')}: {detailMemberRoleSummary.viewer}</Tag> : null}
                                </Space>
                            )}
                            className="workbench-detail-section-card workbench-detail-section-card--secondary"
                        >
                            {relatedMembersQuery.isLoading ? (
                                <Text type="secondary">{t('message.loading', { defaultValue: 'Loading...' })}</Text>
                            ) : detailMemberPreview.length > 0 ? (
                                <div className="entity-detail-list">
                                    {detailMemberPreview.map((member) => {
                                        const primary = member.display_name?.trim() || member.username || member.user_id;
                                        const secondary = member.email || member.username || member.user_id;
                                        return (
                                            <div key={member.user_id} className="entity-detail-list__row entity-detail-list__row--inline">
                                                <Space size={8} wrap className="entity-detail-list__inline-main">
                                                    <Text strong className="entity-detail-list__title">{primary}</Text>
                                                    <Text type="secondary" className="entity-detail-list__inline-separator">·</Text>
                                                    <Text type="secondary" className="entity-detail-list__note">
                                                        {secondary}
                                                    </Text>
                                                </Space>
                                                <Tag color="blue">{t(`role.${member.role}`, { defaultValue: member.role })}</Tag>
                                            </div>
                                        );
                                    })}
                                    {detailMemberCount > detailMemberPreview.length ? (
                                        <Button
                                            type="link"
                                            size="small"
                                            className="entity-detail-list__footnote-action"
                                            onClick={() => setMembersDirectoryOpen(true)}
                                        >
                                            {t('members.detail_more', {
                                                count: detailMemberCount - detailMemberPreview.length,
                                                defaultValue: '+{{count}} more members',
                                            })}
                                        </Button>
                                    ) : null}
                                </div>
                            ) : (
                                <ActionEmptyState
                                    compact={true}
                                    title={t('members.current_members_title', { defaultValue: 'Current Members' })}
                                    description={t('members.empty_description', { defaultValue: 'No members have been assigned to this system yet.' })}
                                    visual={<SystemsOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                />
                            )}
                        </Card>
                    </div>

                    <Card
                        size="small"
                        title={t('systems.related_services_column', 'Services')}
                        extra={<Tag>{detailServiceCount}</Tag>}
                        className="workbench-detail-section-card workbench-detail-section-card--wide"
                    >
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
                    </Card>
                </Space>
            </WorkbenchDetailModal>
            ) : null}

            <Modal
                title={t('members.view_all_modal_title', {
                    name: activeDetailSystem?.name ?? '',
                    defaultValue: 'All members in {{name}}',
                })}
                open={membersDirectoryOpen}
                onCancel={() => {
                    setMembersDirectoryOpen(false);
                    setMembersDirectorySearchDraft('');
                    setMembersDirectorySearch('');
                }}
                footer={[
                    <Button
                        key="close-members-directory"
                        onClick={() => {
                            setMembersDirectoryOpen(false);
                            setMembersDirectorySearchDraft('');
                            setMembersDirectorySearch('');
                        }}
                    >
                        {t('common:button.close', { defaultValue: 'Close' })}
                    </Button>,
                ]}
                width={880}
                destroyOnHidden={true}
            >
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                    <Input.Search
                        allowClear
                        enterButton={t('common:button.search')}
                        placeholder={t('members.current_members_search_placeholder', {
                            defaultValue: 'Search current members by name, email, username, or role',
                        })}
                        value={membersDirectorySearchDraft}
                        onChange={(event) => setMembersDirectorySearchDraft(event.target.value)}
                        onSearch={(value) => setMembersDirectorySearch(value.trim())}
                    />
                    <Table<SystemMember>
                        rowKey="user_id"
                        size="small"
                        dataSource={filteredDetailMembers}
                        pagination={{
                            pageSize: 10,
                            showSizeChanger: true,
                            showTotal: (total) => t('table.total', { total }),
                        }}
                        locale={{
                            emptyText: relatedMembersQuery.isLoading
                                ? t('message.loading', { defaultValue: 'Loading...' })
                                : t('members.empty_description', { defaultValue: 'No members have been assigned to this system yet.' }),
                        }}
                        columns={[
                            {
                                title: t('table.user'),
                                key: 'user',
                                render: (_, member) => (
                                    <div className="entity-detail-list__table-main">
                                        <Text strong>{member.display_name?.trim() || member.username || member.user_id}</Text>
                                        <Text type="secondary">{member.email || member.username || member.user_id}</Text>
                                    </div>
                                ),
                            },
                            {
                                title: t('table.role'),
                                dataIndex: 'role',
                                key: 'role',
                                width: 140,
                                render: (role: SystemMember['role']) => (
                                    <Tag color="blue">{t(`role.${role}`, { defaultValue: role })}</Tag>
                                ),
                            },
                        ]}
                    />
                </Space>
            </Modal>
        </div>
    );
}
