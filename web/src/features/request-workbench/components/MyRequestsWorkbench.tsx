'use client';

import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import {
    AuditOutlined,
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
    Table,
    Tabs,
    Tag,
    Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useRouter, useSearchParams } from 'next/navigation';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';

import { ActionEmptyState } from '@/components/feedback/ActionEmptyState';
import {
    BatchFlowGlyph,
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
    HistoryStatusFilter,
    RequestTicketOperationType,
    RequestWorkbenchView,
} from '../types';
import { STATUS_COLORS } from '../types';

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
    value?: string;
}

interface RequestRowOutcome {
    tone: 'success' | 'warning' | 'danger' | 'muted' | 'info';
    title: string;
    detail?: string;
}

const HISTORY_STATUS_OPTIONS: Array<{ value: HistoryStatusFilter; labelKey: string }> = [
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
                    <Text strong className="workbench-compact-grid__value">
                        {field.value}
                    </Text>
                </div>
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
                    value: requestedShape,
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
                    value: changeSummary,
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
    const detail = record.provisioning?.failure_message?.trim() || record.reject_reason?.trim() || undefined;
    switch (record.status) {
        case 'APPROVED':
            return { tone: 'info', title: t('workbench.outcome.approved_title') };
        case 'EXECUTING':
            return { tone: 'info', title: t('workbench.outcome.executing_title') };
        case 'SUCCESS':
            return { tone: 'success', title: t('workbench.outcome.success_title') };
        case 'FAILED':
            return { tone: 'danger', title: t('workbench.outcome.failed_title'), detail };
        case 'REJECTED':
            return { tone: 'warning', title: t('workbench.outcome.rejected_title'), detail };
        case 'CANCELLED':
            return { tone: 'muted', title: t('workbench.outcome.cancelled_title') };
        default:
            return null;
    }
}

function batchChildSummaryTitle(resourceName: string | undefined): string {
    return resourceName && resourceName.trim() !== '' ? resourceName : EMPTY_VALUE;
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
                description:
                    ticket.provisioning?.failure_message ||
                    ticket.reject_reason ||
                    t('workbench.details.failed_description'),
            };
        case 'REJECTED':
            return {
                type: 'warning' as const,
                message: t('workbench.details.rejected_hint'),
                description: ticket.reject_reason,
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
        {
            key: 'batch_count',
            label: t('workbench.drafts.batch_count'),
            value: requestPrefill.batch_count,
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
        if (
            requestedTab &&
            ['drafts', 'in_progress', 'history', 'batch_jobs'].includes(requestedTab) &&
            requestedTab !== view
        ) {
            changeView(requestedTab as RequestWorkbenchView);
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
    const batchStatusColor =
        requests.batchStatus?.status === 'COMPLETED'
            ? 'green'
            : requests.batchStatus?.status === 'FAILED'
                ? 'red'
                : 'blue';
    const formatBatchStatus = (status?: string) => {
        if (!status) {
            return EMPTY_VALUE;
        }
        const labelKey = `vm:batch.status_value.${status}`;
        const label = t(labelKey);
        return label === labelKey ? status : label;
    };
    const formatBatchOperation = (operation?: string) => {
        if (!operation) {
            return EMPTY_VALUE;
        }
        const labelKey = `vm:batch.operation.${operation}`;
        const label = t(labelKey);
        return label === labelKey ? operation : label;
    };

    const columns: ColumnsType<Ticket> = [
        {
            title: t('request_summary'),
            key: 'request_summary',
            width: 540,
            render: (_, record) => {
                const requestReason = record.reason?.trim();
                const showRequestReason = Boolean(requestReason) && requestReason !== approvalSummaryTitle(record, t);
                const scopeFields = requestScopeFields(record, t);
                const requestedFields = requestFields(record, t);
                return (
                    <Space direction="vertical" size={4} className="workbench-table-stack">
                        <Space size={8} className="workbench-table-heading">
                            <AuditOutlined style={{ color: '#d4380d' }} />
                            <Text strong className="workbench-table-title">
                                {approvalSummaryTitle(record, t)}
                            </Text>
                        </Space>
                        <div className="workbench-table-section-grid">
                            {renderSectionCard(t('workbench.table.scope_label'), scopeFields)}
                            {renderSectionCard(t('workbench.table.request_label'), requestedFields)}
                        </div>
                        {showRequestReason ? (
                            <div className="workbench-inline-meta">
                                <Text type="secondary" className="workbench-inline-meta__label">
                                    {t('reason')}
                                </Text>
                                <Text className="workbench-inline-meta__value">
                                    {requestReason}
                                </Text>
                            </div>
                        ) : null}
                        <Text copyable={{ text: record.id }} type="secondary" className="workbench-ticket-meta">
                            {t('ticket_id')}: {formatApprovalRecordID(record.id)}
                        </Text>
                    </Space>
                );
            },
        },
        {
            title: t('table.progress'),
            key: 'progress',
            width: 260,
            render: (_, record) => {
                const outcome = requestOutcome(record, t);
                return (
                <Space direction="vertical" size={4} className="workbench-table-stack">
                    <Space wrap size={[6, 6]} className="workbench-table-tag-row">
                        <Badge
                            status={
                                record.status === 'PENDING'
                                    ? 'processing'
                                    : record.status === 'APPROVED'
                                        ? 'success'
                                        : 'error'
                            }
                            text={<Tag color={STATUS_COLORS[record.status]}>{t(`status.${record.status}`)}</Tag>}
                        />
                        <Tag color="purple">
                            {record.operation_type ? t(`op_type.${record.operation_type}`) : EMPTY_VALUE}
                        </Tag>
                    </Space>
                    {outcome ? (
                        <div className={`workbench-outcome workbench-outcome--${outcome.tone}`}>
                            <Text strong>{outcome.title}</Text>
                            {outcome.detail ? (
                                <Text className="workbench-table-note">{outcome.detail}</Text>
                            ) : null}
                        </div>
                    ) : null}
                    <div className="workbench-table-meta-stack">
                        <Text type="secondary" className="workbench-table-note">
                            {t('common:table.created_at')}: <LocalDateTimeText value={record.created_at} />
                        </Text>
                        {record.updated_at && record.updated_at !== record.created_at ? (
                            <Text type="secondary" className="workbench-table-note">
                                {t('workbench.details.updated_at')}: <LocalDateTimeText value={record.updated_at} />
                            </Text>
                        ) : null}
                    </div>
                </Space>
            )},
        },
        {
            title: t('common:table.actions'),
            key: 'actions',
            width: 220,
            render: (_, record) => {
                const showReuseAction = (
                    requests.view === 'history' &&
                    record.operation_type === 'CREATE' &&
                    record.request_prefill
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
                        <Space wrap size="small" className="workbench-row-actions">
                            <Button
                                size="small"
                                data-testid={`approval-action-details-${record.id}`}
                                onClick={() => openRequestDetails(record)}
                            >
                                {t('workbench.actions.details')}
                            </Button>
                            <Popover
                                trigger="click"
                                placement="bottomRight"
                                open={openActionMenuId === record.id}
                                onOpenChange={(open) => setOpenActionMenuId(open ? record.id : null)}
                                content={moreContent}
                            >
                                <Button
                                    size="small"
                                    data-testid={`approval-action-more-${record.id}`}
                                    aria-label={`${t('common:table.actions')} ${record.id}`}
                                    icon={<MoreOutlined />}
                                />
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
                        <Space wrap size="small" className="workbench-row-actions">
                            <Button
                                size="small"
                                data-testid={`approval-action-details-${record.id}`}
                                onClick={() => openRequestDetails(record)}
                            >
                                {t('workbench.actions.details')}
                            </Button>
                            <Popover
                                trigger="click"
                                placement="bottomRight"
                                open={openActionMenuId === record.id}
                                onOpenChange={(open) => setOpenActionMenuId(open ? record.id : null)}
                                content={moreContent}
                            >
                                <Button
                                    size="small"
                                    data-testid={`approval-action-more-${record.id}`}
                                    aria-label={`${t('common:table.actions')} ${record.id}`}
                                    icon={<MoreOutlined />}
                                />
                            </Popover>
                        </Space>
                    );
                }

                return (
                    <Button
                        size="small"
                        data-testid={`approval-action-details-${record.id}`}
                        onClick={() => openRequestDetails(record)}
                    >
                        {t('workbench.actions.details')}
                    </Button>
                );
            },
        },
    ];

    const renderRequestTable = () => (
        <Table<Ticket>
            columns={columns}
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
            size="middle"
            scroll={{ x: 960 }}
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

            <div
                className={[
                    'workbench-overview-card',
                    requests.view === 'batch_jobs' ? 'workbench-overview-card--active' : '',
                ].filter(Boolean).join(' ')}
                style={{ '--workbench-overview-accent': '#D66A1F' } as CSSProperties}
            >
                <div className="workbench-overview-card__header">
                    <span className="workbench-overview-card__title">{t('workbench.summary.batch_title')}</span>
                    <BatchFlowGlyph className="workbench-overview-card__art" />
                </div>
                <div className="workbench-overview-card__value">
                    {requests.activeBatchID || t('workbench.summary.batch_inactive')}
                </div>
                <div className="workbench-overview-card__meta">
                    <Space wrap size={8}>
                        <span>{t('workbench.summary.batch_description')}</span>
                        {requests.batchStatus?.status ? (
                            <Tag color={batchStatusColor}>{formatBatchStatus(requests.batchStatus.status)}</Tag>
                        ) : null}
                    </Space>
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

    const renderBatchJobs = () => {
        if (!requests.activeBatchID) {
            return renderEmptyState(
                'workbench.batch_jobs.empty_title',
                'workbench.batch_jobs.empty_description',
                'workbench.open_vms',
                { showSetupGuide: true, visual: <BatchFlowGlyph className="action-empty-state__art" /> },
            );
        }

        return (
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Card
                    variant="borderless"
                    title={t('workbench.batch_jobs.current_title')}
                    extra={(
                        <Space>
                            <Button
                                icon={<ReloadOutlined />}
                                onClick={requests.refreshBatch}
                                loading={requests.batchLoading}
                            >
                                {t('common:button.refresh')}
                            </Button>
                            <Button onClick={requests.clearBatchTracking}>
                                {t('vm:batch.clear')}
                            </Button>
                        </Space>
                    )}
                >
                    <Space direction="vertical" size={16} style={{ width: '100%' }}>
                        <Alert
                            type={requests.batchStatus?.status === 'FAILED' ? 'warning' : 'info'}
                            showIcon
                            message={t('workbench.batch_jobs.status_summary', {
                                status: formatBatchStatus(requests.batchStatus?.status),
                                success: requests.batchStatus?.success_count ?? 0,
                                failed: requests.batchStatus?.failed_count ?? 0,
                                pending: requests.batchStatus?.pending_count ?? 0,
                            })}
                        />
                        <Descriptions bordered size="small" column={2}>
                            <Descriptions.Item label={t('workbench.batch_jobs.batch_id')}>
                                {requests.activeBatchID}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.status')}>
                                <Tag color={
                                    requests.batchStatus?.status === 'COMPLETED'
                                        ? 'green'
                                        : requests.batchStatus?.status === 'FAILED'
                                            ? 'red'
                                            : 'blue'
                                }>
                                    {formatBatchStatus(requests.batchStatus?.status)}
                                </Tag>
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.operation')}>
                                {formatBatchOperation(requests.batchStatus?.operation)}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.child_count')}>
                                {requests.batchStatus?.child_count ?? 0}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.success_count')}>
                                {requests.batchStatus?.success_count ?? 0}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.failed_count')}>
                                {requests.batchStatus?.failed_count ?? 0}
                            </Descriptions.Item>
                            <Descriptions.Item label={t('vm:batch.pending_count')}>
                                {requests.batchStatus?.pending_count ?? 0}
                            </Descriptions.Item>
                        </Descriptions>
                        <Space wrap>
                            <Button
                                onClick={requests.retryBatch}
                                disabled={!requests.batchCanRetry}
                                loading={requests.batchActionPending}
                            >
                                {t('vm:batch.retry_failed')}
                            </Button>
                            <Button
                                danger
                                onClick={requests.cancelBatch}
                                disabled={!requests.batchCanCancel}
                                loading={requests.batchActionPending}
                            >
                                {t('vm:batch.cancel_pending')}
                            </Button>
                            <Button onClick={() => router.push('/vms')}>
                                {t('workbench.open_vms')}
                            </Button>
                        </Space>
                    </Space>
                </Card>

                <Card variant="borderless" title={t('workbench.batch_jobs.child_title')}>
                    <Table
                        rowKey="ticket_id"
                        loading={requests.batchLoading}
                        dataSource={requests.batchStatus?.children ?? []}
                        pagination={false}
                        columns={[
                            {
                                title: t('vm:batch.child.resource'),
                                key: 'resource_summary',
                                render: (_, record) => (
                                    <Space direction="vertical" size={0}>
                                        <Text strong>{batchChildSummaryTitle(record.resource_name)}</Text>
                                        <Text copyable={{ text: record.ticket_id }} type="secondary" style={{ fontSize: 12 }}>
                                            {t('vm:batch.child.ticket')}: {formatApprovalRecordID(record.ticket_id)}
                                        </Text>
                                    </Space>
                                ),
                            },
                            {
                                title: t('vm:batch.child.status'),
                                dataIndex: 'status',
                                key: 'status',
                                render: (status: string) => <Tag>{formatBatchStatus(status)}</Tag>,
                            },
                            { title: t('vm:batch.child.attempt'), dataIndex: 'attempt_count', key: 'attempt_count' },
                            { title: t('vm:batch.child.error'), dataIndex: 'last_error', key: 'last_error' },
                        ]}
                    />
                </Card>
            </Space>
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
                        statusLabel: t(`status.${requests.historyStatus}`),
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
        {
            key: 'batch_jobs',
            label: t('workbench.tab.batch_jobs'),
            children: renderBatchJobs(),
        },
    ];

    return (
        <div data-testid="approvals-page">
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

            <PageSurface>
                <Tabs
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
                                                    key: 'scope',
                                                    width: 280,
                                                    render: (_, record) => (
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
                                                    render: (_, record) => (
                                                        <Space direction="vertical" size={4} className="workbench-batch-cell">
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.current_resources')}
                                                                </Text>
                                                                <Text>{record.currentShape || EMPTY_VALUE}</Text>
                                                            </div>
                                                            <div className="workbench-batch-cell__row">
                                                                <Text type="secondary" className="workbench-batch-cell__label">
                                                                    {t('summary.target_resources')}
                                                                </Text>
                                                                <Text>{record.targetShape || EMPTY_VALUE}</Text>
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
