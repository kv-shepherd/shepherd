'use client';

import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import {
    AuditOutlined,
    EyeOutlined,
    MoreOutlined,
    ReloadOutlined,
} from '@ant-design/icons';
import {
    Alert,
    Badge,
    Button,
    Card,
    Descriptions,
    Drawer,
    Popover,
    Select,
    Segmented,
    Space,
    Tabs,
    Tag,
    List,
    Table,
    Tooltip,
    Typography,
} from 'antd';
import { useRouter, useSearchParams } from 'next/navigation';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import {
    DraftNotebookGlyph,
    QueueReviewGlyph,
    RequestsOverviewGlyph,
} from '@/components/illustrations/DashboardIllustrations';
import { LocalDateTimeText } from '@/components/ui/LocalDateTimeText';
import { PageSearchToolbar } from '@/components/ui/PageSearchToolbar';
import { PageHeader, PageSurface } from '@/components/layouts/PageSection';
import { SetupGuideCard } from '@/features/setup-guide/components/SetupGuideCard';
import { useSetupGuide } from '@/features/setup-guide/hooks/useSetupGuide';
import { useMyRequestsController } from '../hooks/useMyRequestsController';
import {
    approvalEmptyValue,
    approvalPrimaryAlert,
    approvalSummaryTitle,
    buildApprovalBatchDisplayItems,
    buildApprovalChangeItems,
    buildApprovalOverviewItems,
    buildApprovalScopeItems,
    formatApprovalResourceShape,
    formatApprovalRecordID,
} from '@/features/approval-shared/summary';
import type {
    Ticket,
    RequestTicketOperationType,
    RequestWorkbenchView,
    HistoryStatusFilter,
} from '../types';
import { STATUS_COLORS, STATUS_BADGES } from '../types';

const { Text } = Typography;
const EMPTY_VALUE = approvalEmptyValue();

interface RequestContextItem {
    key: string;
    label: string;
    value: string | number;
    isIdentifier?: boolean;
}

interface CompactField {
    label: string;
    value?: ReactNode;
}

interface RequestRowOutcome {
    tone: 'success' | 'warning' | 'danger' | 'muted' | 'info';
    title: string;
    detail?: string;
}

type RequestBatchDisplayRow = ReturnType<typeof buildApprovalBatchDisplayItems>[number];

const HISTORY_STATUS_OPTIONS: Array<{ value: HistoryStatusFilter; labelKey: string }> = [
    { value: 'ALL', labelKey: 'filter_all' },
    { value: 'SUCCESS', labelKey: 'status.SUCCESS' },
    { value: 'FAILED', labelKey: 'status.FAILED' },
    { value: 'REJECTED', labelKey: 'status.REJECTED' },
    { value: 'CANCELLED', labelKey: 'status.CANCELLED' },
];

const filterOptionByLabel = (input: string, option?: { label?: unknown }) => {
    const label = typeof option?.label === 'string' ? option.label : '';
    return label.toLowerCase().includes(input.trim().toLowerCase());
};

function firstVisibleValue(...values: Array<string | undefined | null>): string | undefined {
    for (const value of values) {
        if (typeof value === 'string' && value.trim() !== '') {
            return value.trim();
        }
    }
    return undefined;
}

function renderCompactFieldGrid(fields: CompactField[]) {
    const visibleFields = fields.filter((field) => field.value);
    if (visibleFields.length === 0) {
        return null;
    }
    return (
        <div className="workbench-compact-grid">
            {visibleFields.map((field) => (
                <div key={field.label} className="workbench-compact-grid__item">
                    <Text type="secondary" className="workbench-compact-grid__label">
                        {field.label}
                    </Text>
                    <div className="workbench-compact-grid__value">
                        {typeof field.value === 'string' ? <Text strong>{field.value}</Text> : field.value}
                    </div>
                </div>
            ))}
        </div>
    );
}

function renderResourceShapeFacts(shape: string | undefined) {
    if (!shape || shape.trim() === '') {
        return undefined;
    }
    const parts = shape
        .split('·')
        .map((part) => part.trim())
        .filter((part) => part !== '');
    if (parts.length === 0) {
        return shape;
    }
    return (
        <div className="workbench-resource-facts">
            {parts.map((part) => (
                <span key={part} className="workbench-resource-fact">
                    {part}
                </span>
            ))}
        </div>
    );
}

function renderSectionCard(title: string, fields: CompactField[]) {
    if (fields.every((field) => !field.value)) {
        return null;
    }
    return (
        <div className="workbench-table-section">
            <Text type="secondary" className="workbench-table-section__label">
                {title}
            </Text>
            {renderCompactFieldGrid(fields)}
        </div>
    );
}

function requestScopeFields(record: Ticket, t: TFunction): CompactField[] {
    const summary = record.summary;
    const scope = [
        firstVisibleValue(summary?.system_name, summary?.system_id),
        firstVisibleValue(summary?.service_name, summary?.service_id),
    ].filter(Boolean).join(' / ');
    return [
        { label: t('summary.scope'), value: scope || undefined },
        {
            label: t('summary.namespace'),
            value: firstVisibleValue(summary?.namespace, record.request_prefill?.namespace),
        },
        {
            label: t('summary.cluster'),
            value: firstVisibleValue(summary?.cluster_name, summary?.cluster_id),
        },
        {
            label: t('summary.virtual_machine'),
            value: firstVisibleValue(summary?.vm_name, record.target_vm_name, summary?.vm_id),
        },
    ];
}

function requestFields(record: Ticket, t: TFunction): CompactField[] {
    const summary = record.summary;
    const requestedShape = formatApprovalResourceShape(
        summary?.target_cpu_cores,
        summary?.target_memory_gi,
        summary?.target_disk_gb,
        t,
    );
    const currentShape = formatApprovalResourceShape(
        summary?.current_cpu_cores,
        summary?.current_memory_gi,
        summary?.current_disk_gb,
        t,
    );
    const changeSummary =
        currentShape && requestedShape && currentShape !== requestedShape
            ? `${currentShape} → ${requestedShape}`
            : requestedShape || currentShape;

    switch (record.operation_type) {
        case 'CREATE':
            return [
                {
                    label: t('summary.template'),
                    value: firstVisibleValue(
                        summary?.template_name,
                        summary?.template_id,
                        record.request_prefill?.template_id,
                    ),
                },
                {
                    label: t('summary.instance_size'),
                    value: firstVisibleValue(
                        summary?.instance_size_name,
                        summary?.instance_size_id,
                        record.request_prefill?.instance_size_id,
                    ),
                },
                {
                    label: t('summary.target_resources'),
                    value: renderResourceShapeFacts(requestedShape),
                },
            ];
        case 'MODIFY':
            return [
                {
                    label: t('summary.instance_size'),
                    value: firstVisibleValue(summary?.instance_size_name, summary?.instance_size_id),
                },
                {
                    label: t('summary.target_resources'),
                    value: renderResourceShapeFacts(changeSummary),
                },
            ];
        case 'POWER':
            return [
                {
                    label: t('summary.power_action'),
                    value: firstVisibleValue(summary?.power_action),
                },
                {
                    label: t('summary.virtual_machine_status'),
                    value: firstVisibleValue(summary?.latest_vm_status, summary?.vm_status),
                },
            ];
        case 'DELETE':
        case 'VNC_ACCESS':
            return [
                {
                    label: t('summary.virtual_machine_status'),
                    value: firstVisibleValue(summary?.latest_vm_status, summary?.vm_status),
                },
            ];
        default:
            return [];
    }
}

function requestOutcome(record: Ticket, t: TFunction): RequestRowOutcome | null {
    switch (record.status) {
        case 'APPROVED':
            return { tone: 'info', title: t('workbench.outcome.approved_title') };
        case 'EXECUTING':
            return { tone: 'info', title: t('workbench.outcome.executing_title') };
        case 'SUCCESS':
            return { tone: 'success', title: t('workbench.outcome.success_title') };
        case 'FAILED':
            return { tone: 'danger', title: t('workbench.outcome.failed_title') };
        case 'REJECTED':
            return { tone: 'warning', title: t('workbench.outcome.rejected_title') };
        case 'CANCELLED':
            return { tone: 'muted', title: t('workbench.outcome.cancelled_title') };
        default:
            return null;
    }
}

function requestLifecycleAlert(ticket: Ticket, t: (key: string) => string) {
    const payload = typeof ticket.ticket_payload === 'object' && ticket.ticket_payload !== null
        ? ticket.ticket_payload as Record<string, unknown>
        : undefined;
    const requiresRestart = payload?.requires_restart === true;
    switch (ticket.status) {
        case 'PENDING':
            return {
                type: 'info' as const,
                message: t('workbench.details.pending_hint'),
            };
        case 'APPROVED':
        case 'EXECUTING':
            return {
                type: 'warning' as const,
                message: t('workbench.details.execution_pending_hint'),
                description: t('workbench.details.execution_pending_description'),
            };
        case 'SUCCESS':
            return {
                type: 'success' as const,
                message: t('workbench.details.success_hint'),
                description: requiresRestart
                    ? t('workbench.details.success_restart_required_description')
                    : undefined,
            };
        case 'FAILED':
            return {
                type: 'error' as const,
                message: t('workbench.details.failed_hint'),
                description: t('workbench.details.failed_description'),
            };
        case 'REJECTED':
            return {
                type: 'warning' as const,
                message: t('workbench.details.rejected_hint'),
                description: t('workbench.details.rejected_description'),
            };
        case 'CANCELLED':
            return {
                type: 'info' as const,
                message: t('workbench.details.cancelled_hint'),
            };
        default:
            return null;
    }
}

export function buildRequestWorkbenchDetailItems(
    detailTicket: Ticket | null,
    t: TFunction,
) {
    if (!detailTicket) {
        return [];
    }

    return [
        ...buildApprovalOverviewItems(detailTicket, t),
        { key: 'created', label: t('common:table.created_at'), children: <LocalDateTimeText value={detailTicket.created_at} /> },
        {
            key: 'updated',
            label: t('workbench.details.updated_at'),
            children: detailTicket.updated_at ? <LocalDateTimeText value={detailTicket.updated_at} /> : EMPTY_VALUE,
        },
    ];
}

export function buildRequestWorkbenchContextItems(
    detailTicket: Ticket | null,
    t: TFunction,
): RequestContextItem[] {
    const requestPrefill = detailTicket?.request_prefill;
    const summary = detailTicket?.summary;
    if (!requestPrefill) {
        return [];
    }

    return [
        {
            key: 'system',
            label: t('workbench.details.system'),
            value: summary?.system_name || requestPrefill.system_id || EMPTY_VALUE,
            isIdentifier: !summary?.system_name,
        },
        {
            key: 'service',
            label: t('workbench.details.service'),
            value: summary?.service_name || requestPrefill.service_id || EMPTY_VALUE,
            isIdentifier: !summary?.service_name,
        },
        {
            key: 'template',
            label: t('workbench.drafts.template'),
            value: summary?.template_name || requestPrefill.template_id || EMPTY_VALUE,
            isIdentifier: !summary?.template_name,
        },
        {
            key: 'size',
            label: t('workbench.drafts.size'),
            value: summary?.instance_size_name || requestPrefill.instance_size_id || EMPTY_VALUE,
            isIdentifier: !summary?.instance_size_name,
        },
        {
            key: 'namespace',
            label: t('workbench.drafts.namespace'),
            value: summary?.namespace || requestPrefill.namespace || EMPTY_VALUE,
        },
    ];
}

function renderContextValue(item: RequestContextItem) {
    if (item.isIdentifier && typeof item.value === 'string') {
        return (
            <Text copyable={{ text: item.value }}>
                {formatApprovalRecordID(item.value)}
            </Text>
        );
    }
    return <Text strong>{item.value}</Text>;
}

export function MyRequestsWorkbench() {
    const router = useRouter();
    const searchParams = useSearchParams();
    const { t } = useTranslation(['approval', 'common', 'vm']);
    const requests = useMyRequestsController({ t });
    const setupGuide = useSetupGuide();
    const requestedTab = searchParams.get('tab');
    const { changeView, view } = requests;
    const [detailTicket, setDetailTicket] = useState<Ticket | null>(null);
    const [quickSearchDraft, setQuickSearchDraft] = useState(requests.search);
    const [operationTypeDraft, setOperationTypeDraft] = useState<RequestTicketOperationType | ''>(requests.operationType);
    const [advancedSearchOpen, setAdvancedSearchOpen] = useState(false);
    const [openActionMenuId, setOpenActionMenuId] = useState<string | null>(null);

    useEffect(() => {
        if (!requestedTab || requestedTab === view) {
            return;
        }
        if (['drafts', 'in_progress', 'history'].includes(requestedTab)) {
            changeView(requestedTab as RequestWorkbenchView);
            return;
        }
        if (requestedTab === 'batch_jobs') {
            changeView('in_progress');
        }
    }, [changeView, requestedTab, view]);

    const openRequestDetails = (ticket: Ticket) => {
        setDetailTicket(ticket);
    };

    const closeRequestDetails = () => {
        setDetailTicket(null);
    };

    const closeActionMenu = () => {
        setOpenActionMenuId(null);
    };

    const openRequestContext = (ticket: Ticket) => {
        const params = new URLSearchParams({ request: 'create' });
        if (ticket.request_prefill?.system_id) {
            params.set('system_id', ticket.request_prefill.system_id);
        }
        if (ticket.request_prefill?.service_id) {
            params.set('service_id', ticket.request_prefill.service_id);
        }
        router.push(`/vms?${params.toString()}`);
    };

    const reuseRequest = (ticket: Ticket) => {
        if (requests.prepareHistoryReuse(ticket)) {
            closeRequestDetails();
            router.push('/vms?request=create&draft=resume');
        }
    };

    const approveAlert = useMemo(() => (
        detailTicket ? approvalPrimaryAlert(detailTicket, t) : null
    ), [detailTicket, t]);
    const detailSummaryTitle = useMemo(() => (
        detailTicket ? approvalSummaryTitle(detailTicket, t) : EMPTY_VALUE
    ), [detailTicket, t]);
    const lifecycleAlert = useMemo(() => (
        detailTicket ? requestLifecycleAlert(detailTicket, t as (key: string) => string) : null
    ), [detailTicket, t]);

    const requestDetailItems = useMemo(
        () => buildRequestWorkbenchDetailItems(detailTicket, t),
        [detailTicket, t],
    );

    const requestScopeItems = useMemo(() => (
        detailTicket ? buildApprovalScopeItems(detailTicket, t) : []
    ), [detailTicket, t]);

    const requestChangeItems = useMemo(() => (
        detailTicket ? buildApprovalChangeItems(detailTicket, t) : []
    ), [detailTicket, t]);

    const requestBatchItems = useMemo(() => (
        detailTicket ? buildApprovalBatchDisplayItems(detailTicket, t) : []
    ), [detailTicket, t]);

    const requestContextItems = useMemo(
        () => buildRequestWorkbenchContextItems(detailTicket, t),
        [detailTicket, t],
    );

    const requestListTotal = requests.data?.pagination?.total ?? 0;
    const operationTypeOptions = useMemo(
        () => [
            {
                value: 'CREATE',
                label: t('op_type.CREATE', { defaultValue: 'Create' }),
            },
            {
                value: 'MODIFY',
                label: t('op_type.MODIFY', { defaultValue: 'Modify' }),
            },
            {
                value: 'DELETE',
                label: t('op_type.DELETE', { defaultValue: 'Delete' }),
            },
            {
                value: 'POWER',
                label: t('op_type.POWER', { defaultValue: 'Power' }),
            },
            {
                value: 'VNC_ACCESS',
                label: t('op_type.VNC_ACCESS', { defaultValue: 'VNC Access' }),
            },
        ],
        [t],
    );
    const renderRequestTable = () => (
        <List<Ticket>
            className="workbench-feed-list"
            dataSource={requests.data?.items ?? []}
            rowKey="id"
            loading={requests.isLoading}
            pagination={{
                current: requests.page,
                pageSize: requests.pageSize,
                total: requests.data?.pagination?.total ?? 0,
                showTotal: (total) => t('common:table.total', { total }),
                onChange: (page, pageSize) => {
                    requests.setPage(page);
                    requests.setPageSize(pageSize);
                },
            }}
            renderItem={(record) => {
                const requestReason = record.reason?.trim();
                const showRequestReason = Boolean(requestReason) && requestReason !== approvalSummaryTitle(record, t);
                const scopeFields = requestScopeFields(record, t);
                const requestedFields = requestFields(record, t);
                const outcome = requestOutcome(record, t);
                const showReuseAction = (
                    requests.view === 'history' &&
                    record.operation_type === 'CREATE' &&
                    record.request_prefill
                );

                const renderActions = () => {
                    const detailActionLabel = `${t('workbench.actions.details')} ${record.id}`;
                    const detailAction = (
                        <Tooltip
                            title={t('workbench.actions.details')}
                            trigger={['hover', 'focus']}
                        >
                            <Button
                                type="text"
                                size="small"
                                data-testid={`approval-action-details-${record.id}`}
                                aria-label={detailActionLabel}
                                icon={<EyeOutlined />}
                                onClick={() => openRequestDetails(record)}
                            />
                        </Tooltip>
                    );

                    if (record.status === 'PENDING') {
                        const moreContent = (
                            <div className="workbench-row-menu">
                                <Button
                                    type="text"
                                    danger
                                    block
                                    data-testid={`approval-action-cancel-${record.id}`}
                                    loading={requests.cancelMutation.isPending}
                                    onClick={() => {
                                        closeActionMenu();
                                        requests.cancelMutation.mutate(record.id);
                                    }}
                                >
                                    {t('cancel')}
                                </Button>
                            </div>
                        );
                        return (
                            <Space wrap size="small" className="copy-friendly-actions workbench-row-actions">
                                {detailAction}
                                <Popover
                                    trigger="click"
                                    placement="bottomRight"
                                    open={openActionMenuId === record.id}
                                    onOpenChange={(open) => setOpenActionMenuId(open ? record.id : null)}
                                    content={moreContent}
                                >
                                    <Tooltip
                                        title={t('common:table.actions')}
                                        trigger={['hover', 'focus']}
                                    >
                                        <Button
                                            type="text"
                                            size="small"
                                            data-testid={`approval-action-more-${record.id}`}
                                            aria-label={`${t('common:table.actions')} ${record.id}`}
                                            icon={<MoreOutlined />}
                                        />
                                    </Tooltip>
                                </Popover>
                            </Space>
                        );
                    }
                    if (showReuseAction) {
                        const moreContent = (
                            <div className="workbench-row-menu">
                                <Button
                                    type="text"
                                    block
                                    data-testid={`approval-action-reuse-${record.id}`}
                                    onClick={() => {
                                        closeActionMenu();
                                        reuseRequest(record);
                                    }}
                                >
                                    {t('workbench.history.reuse')}
                                </Button>
                            </div>
                        );
                        return (
                            <Space wrap size="small" className="copy-friendly-actions workbench-row-actions">
                                {detailAction}
                                <Popover
                                    trigger="click"
                                    placement="bottomRight"
                                    open={openActionMenuId === record.id}
                                    onOpenChange={(open) => setOpenActionMenuId(open ? record.id : null)}
                                    content={moreContent}
                                >
                                    <Tooltip
                                        title={t('common:table.actions')}
                                        trigger={['hover', 'focus']}
                                    >
                                        <Button
                                            type="text"
                                            size="small"
                                            data-testid={`approval-action-more-${record.id}`}
                                            aria-label={`${t('common:table.actions')} ${record.id}`}
                                            icon={<MoreOutlined />}
                                        />
                                    </Tooltip>
                                </Popover>
                            </Space>
                        );
                    }
                    return detailAction;
                };

                return (
                    <List.Item className="app-feed-card" style={{ padding: '16px', borderBottom: '1px solid #f0f0f0', display: 'flex', justifyContent: 'space-between' }}>
                        <div className="app-feed-card-main" style={{ display: 'flex', flexDirection: 'column', gap: 8, flex: 1, paddingRight: 24 }}>
                            <Space size={8} className="workbench-table-heading">
                                <AuditOutlined style={{ color: '#d4380d' }} />
                                <Text strong className="workbench-table-title" style={{ fontSize: 15 }}>
                                    {approvalSummaryTitle(record, t)}
                                </Text>
                                <Tag color="purple">
                                    {record.operation_type ? t(`op_type.${record.operation_type}`) : EMPTY_VALUE}
                                </Tag>
                                <Badge
                                    status={STATUS_BADGES[record.status] ?? 'default'}
                                    text={<Text type="secondary" style={{ fontSize: 13, marginLeft: 4 }}>{t(`status.${record.status}`)}</Text>}
                                />
                            </Space>
                            <div className="workbench-table-section-grid" style={{ marginTop: 8 }}>
                                {renderSectionCard(t('workbench.table.scope_label'), scopeFields)}
                                {renderSectionCard(t('workbench.table.request_label'), requestedFields)}
                            </div>
                            {showRequestReason && (
                                <div className="workbench-inline-meta" style={{ marginTop: 4 }}>
                                    <Text type="secondary" className="workbench-inline-meta__label">
                                        {t('reason')}
                                    </Text>
                                    <Text className="workbench-inline-meta__value">
                                        {requestReason}
                                    </Text>
                                </div>
                            )}
                        </div>
                        <div className="app-feed-card-aside" style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', minWidth: 200, gap: 12 }}>
                            {renderActions()}
                            {outcome && (
                                <div className={`workbench-outcome workbench-outcome--${outcome.tone}`} style={{ textAlign: 'right' }}>
                                    <Text strong style={{ display: 'block' }}>{outcome.title}</Text>
                                    {outcome.detail && <Text className="workbench-table-note">{outcome.detail}</Text>}
                                </div>
                            )}
                            <div className="workbench-table-meta-stack" style={{ textAlign: 'right', marginTop: 'auto' }}>
                                <Text copyable={{ text: record.id }} type="secondary" className="workbench-ticket-meta" style={{ display: 'block', fontSize: 12 }}>
                                    ID: {formatApprovalRecordID(record.id)}
                                </Text>
                                <Text type="secondary" className="workbench-table-note" style={{ display: 'block', fontSize: 12 }}>
                                    <LocalDateTimeText value={record.created_at} />
                                </Text>
                                {record.updated_at && record.updated_at !== record.created_at && (
                                    <Text type="secondary" className="workbench-table-note" style={{ display: 'block', fontSize: 12 }}>
                                        {t('workbench.details.updated_at')}: <LocalDateTimeText value={record.updated_at} />
                                    </Text>
                                )}
                            </div>
                        </div>
                    </List.Item>
                );
            }}
        />
    );

    const renderRequestSearchToolbar = (includeHistoryStatus: boolean) => (
        <PageSearchToolbar
            searchValue={requests.search}
            searchDraftValue={quickSearchDraft}
            onSearchDraftChange={setQuickSearchDraft}
            onSearchChange={(value) => {
                const nextValue = value.trim();
                setQuickSearchDraft(nextValue);
                requests.applySearch(nextValue);
            }}
            searchPlaceholder={t('workbench.search_placeholder', {
                defaultValue: 'Search requests by reason, cluster, requester, or paste a ticket ID',
            })}
            searchTestId="workbench-quick-search"
            searchHelp={t('workbench.search_help', {
                defaultValue:
                    'Press Enter or click Search. Quick search matches reasons, requester, cluster name, and pasted ticket IDs.',
            })}
            secondaryActions={includeHistoryStatus ? (
                <Segmented
                    data-testid="approvals-status-filter"
                    value={requests.historyStatus}
                    onChange={(value) => requests.changeHistoryStatus(value as HistoryStatusFilter)}
                    options={HISTORY_STATUS_OPTIONS.map((option) => ({
                        label: t(option.labelKey),
                        value: option.value,
                    }))}
                />
            ) : undefined}
            advancedSearch={{
                open: advancedSearchOpen,
                onToggle: () => setAdvancedSearchOpen((open) => !open),
                openLabel: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                closeLabel: t('common:search.hide_advanced', { defaultValue: 'Hide advanced search' }),
                title: t('common:search.advanced', { defaultValue: 'Advanced search' }),
                toggleTestId: 'workbench-advanced-search-toggle',
                content: (
                    <Space direction="vertical" size={12} style={{ width: '100%' }}>
                        <Text type="secondary">
                            {t('workbench.advanced_search_help', {
                                defaultValue:
                                    'Choose exact filters here. Options support keyword matching, but the applied filter remains an exact value.',
                            })}
                        </Text>
                        <Space wrap size={[12, 12]}>
                            <Select
                                allowClear
                                showSearch
                                filterOption={filterOptionByLabel}
                                optionFilterProp="label"
                                style={{ minWidth: 240 }}
                                data-testid="workbench-filter-operation"
                                placeholder={t('operation_type')}
                                options={operationTypeOptions}
                                value={operationTypeDraft || undefined}
                                onChange={(value) => setOperationTypeDraft((value as RequestTicketOperationType | undefined) ?? '')}
                            />
                            <Button
                                type="primary"
                                data-testid="workbench-advanced-search-submit"
                                onClick={() => requests.applyOperationType(operationTypeDraft)}
                            >
                                {t('common:button.search')}
                            </Button>
                        </Space>
                    </Space>
                ),
            }}
            hasActiveFilters={requests.search !== '' || requests.operationType !== ''}
            onClear={() => {
                setQuickSearchDraft('');
                setOperationTypeDraft('');
                setAdvancedSearchOpen(false);
                requests.clearListFilters();
            }}
            clearLabel={t('common:button.clear_filters', { defaultValue: 'Clear filters' })}
        />
    );

    const renderQueueBanner = (options: {
        title: string;
        description: string;
        count?: number;
        statusLabel?: string;
    }) => (
        <div className="workbench-queue-banner">
            <div className="workbench-queue-banner__header">
                <Space wrap size={8}>
                    <Text strong>{options.title}</Text>
                    {typeof options.count === 'number' ? <Tag>{t('common:table.total', { total: options.count })}</Tag> : null}
                    {options.statusLabel ? <Tag color="blue">{options.statusLabel}</Tag> : null}
                </Space>
            </div>
            <Text type="secondary">{options.description}</Text>
        </div>
    );

    const renderEmptyState = (
        titleKey: string,
        descriptionKey: string,
        buttonKey: string,
        options?: {
            showSetupGuide?: boolean;
            visual?: ReactNode;
        },
    ) => (
        options?.showSetupGuide && !setupGuide.vmRequestReady ? (
            <div style={{ padding: 24 }}>
                <SetupGuideCard variant="vm" surface={false} />
            </div>
        ) : (
            <div style={{ padding: 32 }}>
                <ActionEmptyState
                    title={t(titleKey)}
                    description={t(descriptionKey)}
                    visual={options?.visual}
                    actions={(
                    <Button type="primary" onClick={() => router.push('/vms?request=create')}>
                        {t(buttonKey)}
                    </Button>
                    )}
                />
            </div>
        )
    );

    const renderSummaryCards = () => (
        <div className="workbench-overview-strip">
            <div
                className={[
                    'workbench-overview-card',
                    requests.savedVmDraft ? 'workbench-overview-card--active' : '',
                ].filter(Boolean).join(' ')}
                style={{ '--workbench-overview-accent': '#1D5BFF' } as CSSProperties}
            >
                <div className="workbench-overview-card__header">
                    <span className="workbench-overview-card__title">{t('workbench.summary.draft_title')}</span>
                    <DraftNotebookGlyph className="workbench-overview-card__art" />
                </div>
                <div className="workbench-overview-card__value">
                    {requests.savedVmDraft ? t('workbench.summary.draft_ready') : t('workbench.summary.draft_empty')}
                </div>
                <div className="workbench-overview-card__meta">
                    {requests.savedVmDraft?.updatedAt ? (
                        <span>
                            {t('workbench.drafts.updated_at')}: <LocalDateTimeText value={requests.savedVmDraft.updatedAt} />
                        </span>
                    ) : (
                        <span>{t('workbench.summary.draft_description')}</span>
                    )}
                </div>
                <div className="workbench-overview-card__actions">
                    <Button
                        size="small"
                        type={requests.savedVmDraft ? 'primary' : 'default'}
                        onClick={() => router.push(requests.savedVmDraft ? '/vms?request=create&draft=resume' : '/vms?request=create')}
                    >
                        {t('workbench.summary.draft_cta')}
                    </Button>
                </div>
            </div>

            <div
                className={[
                    'workbench-overview-card',
                    requests.view === 'in_progress' ? 'workbench-overview-card--active' : '',
                ].filter(Boolean).join(' ')}
                style={{ '--workbench-overview-accent': '#D97706' } as CSSProperties}
            >
                <div className="workbench-overview-card__header">
                    <span className="workbench-overview-card__title">{t('workbench.summary.in_progress_title')}</span>
                    <QueueReviewGlyph className="workbench-overview-card__art" />
                </div>
                <div className="workbench-overview-card__value">
                    {requests.view === 'in_progress' ? requestListTotal : t('workbench.summary.load_on_open_short')}
                </div>
                <div className="workbench-overview-card__meta">
                    {requests.view === 'in_progress'
                        ? t('workbench.summary.pending_description', { count: requestListTotal })
                        : t('workbench.summary.load_on_open')}
                </div>
            </div>

            <div
                className={[
                    'workbench-overview-card',
                    requests.view === 'history' ? 'workbench-overview-card--active' : '',
                ].filter(Boolean).join(' ')}
                style={{ '--workbench-overview-accent': '#7C3AED' } as CSSProperties}
            >
                <div className="workbench-overview-card__header">
                    <span className="workbench-overview-card__title">{t('workbench.summary.history_title')}</span>
                    <RequestsOverviewGlyph className="workbench-overview-card__art" />
                </div>
                <div className="workbench-overview-card__value">
                    {requests.view === 'history' ? requestListTotal : t('workbench.summary.load_on_open_short')}
                </div>
                <div className="workbench-overview-card__meta">
                    {requests.view === 'history'
                        ? t('workbench.summary.history_description', { count: requestListTotal })
                        : t('workbench.summary.load_on_open')}
                </div>
            </div>
        </div>
    );

    const renderDrafts = () => {
        if (!requests.savedVmDraft) {
            return renderEmptyState(
                'workbench.drafts.empty_title',
                'workbench.drafts.empty_description',
                'workbench.open_vms',
                { showSetupGuide: true },
            );
        }

        return (
            <Card variant="borderless" styles={{ body: { padding: 0 } }}>
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                    <Space direction="vertical" size={4}>
                        <Text strong>{t('workbench.drafts.saved_title')}</Text>
                        <Text type="secondary">{t('workbench.drafts.saved_description')}</Text>
                    </Space>
                    <Descriptions bordered size="small" column={2}>
                        <Descriptions.Item label={t('workbench.drafts.service')}>
                            {requests.savedVmDraft.serviceLabel || requests.savedVmDraft.serviceId || EMPTY_VALUE}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.template')}>
                            {requests.savedVmDraft.templateLabel || requests.savedVmDraft.templateId || EMPTY_VALUE}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.size')}>
                            {requests.savedVmDraft.instanceSizeLabel || requests.savedVmDraft.instanceSizeId || EMPTY_VALUE}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.namespace')}>
                            {requests.savedVmDraft.namespace || EMPTY_VALUE}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.batch_count')}>
                            {requests.savedVmDraft.batchCount}
                        </Descriptions.Item>
                        <Descriptions.Item label={t('workbench.drafts.updated_at')}>
                            <LocalDateTimeText value={requests.savedVmDraft.updatedAt} />
                        </Descriptions.Item>
                    </Descriptions>
                    <Space wrap>
                        <Button type="primary" onClick={() => router.push('/vms?request=create&draft=resume')}>
                            {t('workbench.drafts.resume')}
                        </Button>
                        <Button onClick={requests.discardSavedVmDraft}>
                            {t('workbench.drafts.discard')}
                        </Button>
                    </Space>
                </Space>
            </Card>
        );
    };

    const tabItems: Array<{ key: RequestWorkbenchView; label: string; children: ReactNode }> = [
        {
            key: 'drafts',
            label: t('workbench.tab.drafts'),
            children: renderDrafts(),
        },
        {
            key: 'in_progress',
            label: t('workbench.tab.in_progress'),
            children: (
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                    {renderQueueBanner({
                        title: t('workbench.summary.in_progress_title'),
                        description: t('workbench.in_progress.description'),
                        count: requestListTotal,
                    })}
                    {renderRequestSearchToolbar(false)}
                    {(requests.data?.items?.length ?? 0) === 0 && !requests.isLoading
                        ? renderEmptyState(
                            'workbench.in_progress.empty_title',
                            'workbench.in_progress.empty_description',
                            'workbench.open_vms',
                            { showSetupGuide: true, visual: <QueueReviewGlyph className="action-empty-state__art" /> },
                        )
                        : renderRequestTable()}
                </Space>
            ),
        },
        {
            key: 'history',
            label: t('workbench.tab.history'),
            children: (
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                    {renderQueueBanner({
                        title: t('workbench.summary.history_title'),
                        description: t('workbench.history.description'),
                        count: requestListTotal,
                        statusLabel: requests.historyStatus === 'ALL' ? t('filter_all') : t(`status.${requests.historyStatus}`),
                    })}
                    {renderRequestSearchToolbar(true)}
                    {(requests.data?.items?.length ?? 0) === 0 && !requests.isLoading
                        ? renderEmptyState(
                            'workbench.history.empty_title',
                            'workbench.history.empty_description',
                            'workbench.open_vms',
                            { showSetupGuide: true, visual: <RequestsOverviewGlyph className="action-empty-state__art" /> },
                        )
                        : renderRequestTable()}
                </Space>
            ),
        },
    ];

    return (
        <div data-testid="approvals-page" className="request-workbench-page">
            {requests.messageContextHolder}
            <PageHeader
                title={t('my_approvals_title')}
                subtitle={t('my_approvals_subtitle')}
                actions={(
                    <Space>
                    {(requests.view === 'in_progress' || requests.view === 'history') && (
                        <Button icon={<ReloadOutlined />} onClick={() => requests.refetch()}>
                            {t('common:button.refresh')}
                        </Button>
                    )}
                    </Space>
                )}
            />

            {renderSummaryCards()}

            <PageSurface className="request-workbench-page__tabs-surface">
                <Tabs
                    className="request-workbench-page__tabs"
                    activeKey={requests.view}
                    onChange={(key) => requests.changeView(key as RequestWorkbenchView)}
                    items={tabItems}
                />
            </PageSurface>

            {detailTicket ? (
                <Drawer
                    title={(
                        <Space wrap>
                            <span>{t('workbench.details.title')}</span>
                            <Tag color={STATUS_COLORS[detailTicket.status]}>
                                {t(`status.${detailTicket.status}`)}
                            </Tag>
                            {detailTicket.operation_type ? (
                                <Tag color="purple">{t(`op_type.${detailTicket.operation_type}`)}</Tag>
                            ) : null}
                        </Space>
                    )}
                    open={true}
                    onClose={closeRequestDetails}
                    width="min(1040px, calc(100vw - 16px))"
                    styles={{ body: { paddingRight: 8 } }}
                    className="request-workbench-page__drawer"
                    footer={(
                        <Space wrap>
                            <Button onClick={closeRequestDetails}>
                                {t('common:button.close')}
                            </Button>
                            {detailTicket.request_prefill && (
                                <Button onClick={() => openRequestContext(detailTicket)}>
                                    {t('workbench.details.open_request_context')}
                                </Button>
                            )}
                            {detailTicket.status === 'PENDING' && (
                                <Button
                                    danger
                                    loading={requests.cancelMutation.isPending}
                                    onClick={() => {
                                        requests.cancelMutation.mutate(detailTicket.id);
                                        closeRequestDetails();
                                    }}
                                >
                                    {t('cancel')}
                                </Button>
                            )}
                            {requests.view === 'history' &&
                                detailTicket.operation_type === 'CREATE' &&
                                detailTicket.request_prefill && (
                                <Button
                                    type="primary"
                                    onClick={() => reuseRequest(detailTicket)}
                                >
                                    {t('workbench.history.reuse')}
                                </Button>
                            )}
                        </Space>
                    )}
                >
                    <div className="workbench-detail-modal__viewport" style={{ maxHeight: 'calc(100vh - 150px)' }}>
                        <div className="workbench-detail-modal__canvas" style={{ minWidth: 900 }}>
                            <Space direction="vertical" size={16} className="workbench-detail-modal__stack">
                            {approveAlert ? (
                                <Alert
                                    type={approveAlert.type}
                                    showIcon
                                    message={approveAlert.message}
                                    description={approveAlert.description}
                                />
                            ) : null}
                            {lifecycleAlert ? (
                                <Alert
                                    type={lifecycleAlert.type}
                                    showIcon
                                    message={lifecycleAlert.message}
                                    description={lifecycleAlert.description}
                                />
                            ) : null}
                            <Card size="small" className="workbench-detail-hero">
                                <Space direction="vertical" size={6} style={{ width: '100%' }}>
                                    <Space wrap size={8}>
                                        <Text strong style={{ fontSize: 16 }}>{detailSummaryTitle}</Text>
                                        <Tag color={STATUS_COLORS[detailTicket.status]}>
                                            {t(`status.${detailTicket.status}`)}
                                        </Tag>
                                        {detailTicket.operation_type ? (
                                            <Tag color="purple">{t(`op_type.${detailTicket.operation_type}`)}</Tag>
                                        ) : null}
                                    </Space>
                                    <Space wrap size={10} className="workbench-detail-hero__meta">
                                        <Text copyable={{ text: detailTicket.id }} type="secondary" className="workbench-ticket-meta">
                                            {t('ticket_id')}: {formatApprovalRecordID(detailTicket.id)}
                                        </Text>
                                        <Text type="secondary">
                                            {t('common:table.created_at')}: <LocalDateTimeText value={detailTicket.created_at} />
                                        </Text>
                                        <Text type="secondary">
                                            {t('workbench.details.updated_at')}: {detailTicket.updated_at ? <LocalDateTimeText value={detailTicket.updated_at} /> : EMPTY_VALUE}
                                        </Text>
                                    </Space>
                                    <div className="workbench-detail-hero__grid">
                                        {renderSectionCard(t('workbench.table.scope_label'), requestScopeFields(detailTicket, t))}
                                        {renderSectionCard(t('workbench.table.request_label'), requestFields(detailTicket, t))}
                                    </div>
                                </Space>
                            </Card>
                            <div className="workbench-detail-modal__grid">
                                {requestScopeItems.length > 0 && (
                                    <Card
                                        variant="borderless"
                                        className="workbench-detail-section-card workbench-detail-section-card--primary"
                                        title={t('workbench.details.resource_title')}
                                    >
                                        <Descriptions
                                            bordered
                                            size="small"
                                            column={1}
                                            items={requestScopeItems}
                                        />
                                    </Card>
                                )}
                                {requestChangeItems.length > 0 && (
                                    <Card
                                        variant="borderless"
                                        className="workbench-detail-section-card workbench-detail-section-card--secondary"
                                        title={t('workbench.details.change_title')}
                                    >
                                        <Descriptions
                                            bordered
                                            size="small"
                                            column={1}
                                            items={requestChangeItems}
                                        />
                                    </Card>
                                )}
                                    <Card
                                        variant="borderless"
                                        className="workbench-detail-section-card workbench-detail-section-card--tertiary"
                                        title={t('workbench.details.workflow_title')}
                                    >
                                    <Descriptions
                                        bordered
                                        size="small"
                                        column={1}
                                        items={requestDetailItems}
                                    />
                                </Card>
                            </div>
                            {requestBatchItems.length > 0 && (
                                <Card
                                    variant="borderless"
                                    className="workbench-detail-section-card workbench-detail-section-card--wide"
                                    title={t('workbench.details.affected_items_title')}
                                >
                                    <div className="workbench-detail-modal__table-scroll">
                                        <Table
                                            rowKey="key"
                                            size="small"
                                            pagination={false}
                                            scroll={{ x: 760 }}
                                            dataSource={requestBatchItems}
                                            columns={[
                                                {
                                                    title: t('summary.item'),
                                                    dataIndex: 'title',
                                                    key: 'title',
                                                    width: 240,
                                                    render: (value: string | undefined) => (
                                                        <Space direction="vertical" size={4} className="workbench-table-stack">
                                                            <Text strong className="workbench-table-title">
                                                                {value || EMPTY_VALUE}
                                                            </Text>
                                                        </Space>
                                                    ),
                                                },
                                                {
                                                    title: t('summary.scope'),
                                                    width: 280,
                                                    render: (_: unknown, record: RequestBatchDisplayRow) => (
                                                        <Space direction="vertical" size={4} className="workbench-batch-cell">
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.scope')}
                                                                </Text>
                                                                <Text>{record.scope || EMPTY_VALUE}</Text>
                                                            </div>
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.cluster')}
                                                                </Text>
                                                                <Text>{record.cluster || EMPTY_VALUE}</Text>
                                                            </div>
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.request_vm_status')}
                                                                </Text>
                                                                <Text>{record.requestStatus || EMPTY_VALUE}</Text>
                                                            </div>
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.latest_vm_status')}
                                                                </Text>
                                                                <Space direction="vertical" size={0}>
                                                                    <Text>{record.latestStatus || EMPTY_VALUE}</Text>
                                                                </Space>
                                                            </div>
                                                            {record.statusChanged ? (
                                                                <Text type="warning" className="workbench-table-note">
                                                                    {t('summary.status_changed')}
                                                                </Text>
                                                            ) : null}
                                                        </Space>
                                                    ),
                                                },
                                                {
                                                    title: t('summary.target_resources'),
                                                    key: 'target_resources',
                                                    width: 280,
                                                    render: (_: unknown, record: RequestBatchDisplayRow) => (
                                                        <Space direction="vertical" size={4} className="workbench-batch-cell">
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.current_resources')}
                                                                </Text>
                                                                <div>{renderResourceShapeFacts(record.currentShape) || <Text>{EMPTY_VALUE}</Text>}</div>
                                                            </div>
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.target_resources')}
                                                                </Text>
                                                                <div>{renderResourceShapeFacts(record.targetShape) || <Text>{EMPTY_VALUE}</Text>}</div>
                                                            </div>
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.power_action')}
                                                                </Text>
                                                                <Text>{record.action || EMPTY_VALUE}</Text>
                                                            </div>
                                                        </Space>
                                                    ),
                                                },
                                            ]}
                                        />
                                    </div>
                                </Card>
                            )}
                            <Card
                                variant="borderless"
                                className="workbench-detail-section-card workbench-detail-section-card--wide"
                                title={t('workbench.details.context_title')}
                                extra={detailTicket?.request_prefill ? (
                                    <Text type="secondary">{t('workbench.details.context_description')}</Text>
                                ) : undefined}
                            >
                                {detailTicket?.request_prefill ? (
                                    <div className="workbench-context-grid">
                                        {requestContextItems.map((item) => (
                                            <div key={item.key} className="workbench-context-card">
                                                <Text type="secondary" className="workbench-context-card__label">
                                                    {item.label}
                                                </Text>
                                                <div className="workbench-context-card__value">
                                                    {renderContextValue(item)}
                                                </div>
                                            </div>
                                        ))}
                                    </div>
                                ) : (
                                    <ActionEmptyState
                                        compact={true}
                                        title={t('workbench.details.context_title')}
                                        description={t('workbench.details.no_context')}
                                        visual={<RequestsOverviewGlyph className="action-empty-state__art action-empty-state__art--compact" />}
                                    />
                                )}
                            </Card>
                            </Space>
                        </div>
                    </div>
                </Drawer>
            ) : null}
        </div>
    );
}
