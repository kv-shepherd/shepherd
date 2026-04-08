import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { TFunction } from 'i18next';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const state = vi.hoisted(() => ({
    overrides: {} as Record<string, unknown>,
    push: vi.fn(),
    searchParams: new URLSearchParams(),
}));

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        push: state.push,
    }),
    useSearchParams: () => state.searchParams,
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { count?: number; index?: number }) => {
            const labels: Record<string, string> = {
                my_approvals_title: 'My Requests',
                my_approvals_subtitle: 'Track active requests, history, and upcoming recovery tools',
                'workbench.summary.batch_title': 'Batch Tracking',
                'workbench.summary.batch_inactive': 'No active batch',
                'workbench.summary.batch_description': 'Track the latest parent-child execution without returning to the VM page.',
                'workbench.tab.drafts': 'Drafts',
                'workbench.tab.in_progress': 'In Progress',
                'workbench.tab.history': 'History',
                'workbench.tab.batch_jobs': 'Batch Jobs',
                'workbench.drafts.empty_title': 'No saved drafts yet',
                'workbench.batch_jobs.current_title': 'Current Batch Job',
                'workbench.batch_jobs.child_title': 'Child Tasks',
                'workbench.batch_jobs.batch_id': 'Batch ID',
                'workbench.batch_jobs.retry_submitted': 'Retry submitted',
                'workbench.batch_jobs.cancel_submitted': 'Cancel submitted',
                'workbench.drafts.saved_title': 'Saved VM request draft',
                'workbench.drafts.saved_description': 'This draft is stored locally and can be resumed from the VM request flow.',
                'workbench.drafts.service': 'Service',
                'workbench.drafts.template': 'Template',
                'workbench.drafts.size': 'Instance Size',
                'workbench.drafts.namespace': 'Namespace',
                'workbench.drafts.batch_count': 'Batch Count',
                'workbench.drafts.updated_at': 'Updated',
                'workbench.drafts.resume': 'Resume Draft',
                'workbench.drafts.discard': 'Discard Draft',
                'workbench.open_vms': 'Open Virtual Machines',
                'workbench.in_progress.empty_title': 'No active requests yet',
                'workbench.in_progress.empty_description': 'Submit the first VM request to start tracking approvals and execution here.',
                'workbench.history.description': 'Browse completed request outcomes by decision status.',
                'workbench.search_placeholder': 'Search requests by reason, cluster, requester, or paste a ticket ID',
                'workbench.search_help': 'Press Enter or click Search. Quick search matches reasons, requester, cluster name, and pasted ticket IDs.',
                'workbench.advanced_search_help': 'Choose exact filters here. Options support keyword matching, but the applied filter remains an exact value.',
                'workbench.history.empty_title': 'No request history yet',
                'workbench.history.empty_description': 'Completed requests will appear here after the first workflow finishes.',
                'workbench.history.reuse': 'Reuse Request',
                'workbench.actions.details': 'Details',
                'workbench.details.title': 'Request Details',
                'workbench.details.summary_title': 'Request Summary',
                'workbench.details.resource_title': 'Resource Context',
                'workbench.details.change_title': 'Requested Change',
                'workbench.details.affected_items_title': 'Affected Items',
                'workbench.details.context_title': 'Recovered Request Context',
                'workbench.details.updated_at': 'Updated',
                'workbench.details.system': 'System',
                'workbench.details.service': 'Service',
                'workbench.details.no_context': 'This request does not have reusable request context.',
                'workbench.details.open_request_context': 'Open Request Context',
                'workbench.details.execution_pending_hint': 'Approval is complete, but platform execution is still in progress.',
                'workbench.details.execution_pending_description': 'Do not treat this as a finished change yet. The downstream VM operation can still fail later and will show its final outcome here.',
                'workbench.details.success_hint': 'This request finished successfully.',
                'workbench.details.failed_hint': 'Approval was accepted, but downstream execution failed.',
                'workbench.details.failed_description': 'Review the failure details below before retrying or submitting a follow-up request.',
                'workbench.details.rejected_hint': 'This request was rejected and will not execute.',
                'workbench.details.cancelled_hint': 'This request was cancelled before execution completed.',
                'workbench.in_progress.description': 'Review requests that are still waiting for approval or downstream processing.',
                ticket_id: 'Ticket ID',
                operation_type: 'Operation',
                reason: 'Reason',
                requester: 'Requester',
                approver: 'Approver',
                cancel: 'Cancel Request',
                'common:table.status': 'Status',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'common:button.refresh': 'Refresh',
                'common:button.search': 'Search',
                'common:button.clear_filters': 'Clear filters',
                'common:button.close': 'Close',
                'common:table.total': 'Total',
                'common:search.advanced': 'Advanced search',
                'common:search.hide_advanced': 'Hide advanced search',
                'status.PENDING': 'Pending',
                'status.APPROVED': 'Approved',
                'status.REJECTED': 'Rejected',
                'status.CANCELLED': 'Cancelled',
                'status.SUCCESS': 'Success',
                'status.FAILED': 'Failed',
                'op_type.CREATE': 'Create',
                'op_type.MODIFY': 'Modify',
                'op_type.DELETE': 'Delete',
                'op_type.POWER': 'Power',
                'op_type.VNC_ACCESS': 'VNC Access',
                'vm:batch.clear': 'Clear',
                'vm:batch.status': 'Status',
                'vm:batch.operation': 'Operation',
                'vm:batch.operation.CREATE': 'Create',
                'vm:batch.operation.MODIFY': 'Modify',
                'vm:batch.operation.DELETE': 'Delete',
                'vm:batch.operation.START': 'Start',
                'vm:batch.operation.STOP': 'Stop',
                'vm:batch.operation.RESTART': 'Restart',
                'vm:batch.child_count': 'Child Count',
                'vm:batch.success_count': 'Success',
                'vm:batch.failed_count': 'Failed',
                'vm:batch.pending_count': 'Pending',
                'vm:batch.retry_failed': 'Retry Failed',
                'vm:batch.cancel_pending': 'Cancel Pending',
                'vm:batch.child.ticket': 'Ticket ID',
                'vm:batch.child.resource': 'Resource',
                'vm:batch.child.status': 'Status',
                'vm:batch.child.attempt': 'Attempts',
                'vm:batch.child.error': 'Last Error',
                'summary.system': 'System',
                'summary.service': 'Service',
                'summary.namespace': 'Namespace',
                'summary.cluster': 'Cluster',
                'summary.virtual_machine': 'Virtual Machine',
                'summary.virtual_machine_status': 'VM Status',
                'summary.request_vm_status': 'Request-Time Status',
                'summary.latest_vm_status': 'Latest Status',
                'summary.status_changed': 'Changed since request',
                'summary.template': 'Template',
                'summary.instance_size': 'Instance Size',
                'summary.current_resources': 'Current Resources',
                'summary.target_resources': 'Requested Resources',
                'summary.power_action': 'Requested Action',
                'summary.item': 'Request',
                'summary.scope': 'Scope',
                request_batch_count: '{{count}} VM requests',
            };
            if (key === 'summary.shape_cpu' && typeof options?.count === 'number') {
                return `${options.count} vCPU`;
            }
            if (key === 'summary.shape_memory' && typeof options?.count === 'number') {
                return `${options.count} Gi memory`;
            }
            if (key === 'summary.shape_disk' && typeof options?.count === 'number') {
                return `${options.count} Gi disk`;
            }
            if (key === 'request_batch_count' && typeof options?.count === 'number') {
                return `${options.count} VM requests`;
            }
            if (key === 'summary.item_fallback' && typeof options?.index === 'number') {
                return `Request #${options.index}`;
            }
            return labels[key] ?? key;
        },
    }),
}));

vi.mock('@/features/setup-guide/hooks/useSetupGuide', () => ({
    useSetupGuide: () => ({
        systemsTotal: 1,
        servicesTotal: 1,
        vmsTotal: 0,
        namespacesTotal: 1,
        templatesTotal: 1,
        instanceSizesTotal: 1,
        canCreateSystem: true,
        canCreateService: true,
        canCreateVM: true,
        canManageNamespaces: true,
        canManageTemplates: true,
        canManageInstanceSizes: true,
        systemReady: true,
        serviceReady: true,
        prerequisitesReady: true,
        vmRequestReady: true,
        hasRequestedFirstVM: false,
        isLoading: false,
    }),
}));

vi.mock('@/features/setup-guide/components/SetupGuideCard', () => ({
    SetupGuideCard: ({ variant }: { variant: string }) => <div>{`setup-guide-${variant}`}</div>,
}));

vi.mock('@/components/feedback/ActionEmptyState', () => ({
    ActionEmptyState: ({
        title,
        description,
        actions,
    }: {
        title: string;
        description?: string;
        actions?: ReactNode;
    }) => (
        <section data-testid="action-empty-state">
            <h2>{title}</h2>
            {description ? <p>{description}</p> : null}
            {actions}
        </section>
    ),
}));

vi.mock('@/components/feedback/SummaryMetricCard', () => ({
    SummaryMetricCard: ({
        title,
        value,
        description,
        action,
    }: {
        title: ReactNode;
        value?: ReactNode;
        description?: ReactNode;
        action?: ReactNode;
    }) => (
        <section data-testid="summary-metric-card">
            <h2>{title}</h2>
            {value ? <div>{value}</div> : null}
            {description ? <div>{description}</div> : null}
            {action}
        </section>
    ),
}));

vi.mock('@/components/layouts/PageSection', () => ({
    PageHeader: ({
        title,
        subtitle,
        actions,
    }: {
        title: ReactNode;
        subtitle?: ReactNode;
        actions?: ReactNode;
    }) => (
        <header data-testid="page-header">
            <h1>{title}</h1>
            {subtitle ? <p>{subtitle}</p> : null}
            {actions}
        </header>
    ),
    PageSurface: ({ children }: { children: ReactNode }) => (
        <section data-testid="page-surface">{children}</section>
    ),
}));

vi.mock('@/components/ui/LocalDateTimeText', () => ({
    LocalDateTimeText: ({ value }: { value: string }) => (
        <time dateTime={value}>{value}</time>
    ),
}));

vi.mock('@/components/illustrations/DashboardIllustrations', () => ({
    BatchFlowGlyph: (props: Record<string, unknown>) => <span {...props}>batch-glyph</span>,
    DraftNotebookGlyph: (props: Record<string, unknown>) => <span {...props}>draft-glyph</span>,
    QueueReviewGlyph: (props: Record<string, unknown>) => <span {...props}>queue-glyph</span>,
    RequestsOverviewGlyph: (props: Record<string, unknown>) => <span {...props}>requests-glyph</span>,
}));

vi.mock('antd', async () => {
    const actual = await vi.importActual<typeof import('antd')>('antd');

    const Card = ({
        children,
        title,
        extra,
    }: {
        children?: ReactNode;
        title?: ReactNode;
        extra?: ReactNode;
    }) => (
        <section data-testid="mock-card">
            {title ? <div>{title}</div> : null}
            {extra ? <div>{extra}</div> : null}
            {children}
        </section>
    );

    const DescriptionsItem = ({
        label,
        children,
    }: {
        label?: ReactNode;
        children?: ReactNode;
    }) => (
        <div data-testid="mock-description-item">
            {label ? <dt>{label}</dt> : null}
            <dd>{children}</dd>
        </div>
    );

    const Descriptions = (({ children }: { children?: ReactNode }) => (
        <dl data-testid="mock-descriptions">{children}</dl>
    )) as ((props: { children?: ReactNode }) => ReactNode) & { Item: typeof DescriptionsItem };
    Descriptions.Item = DescriptionsItem;

    const Drawer = ({
        open,
        title,
        children,
        footer,
    }: {
        open?: boolean;
        title?: ReactNode;
        children?: ReactNode;
        footer?: ReactNode;
    }) =>
        open ? (
            <section data-testid="mock-drawer">
                {title ? <div>{title}</div> : null}
                <div>{children}</div>
                {footer ? <div>{footer}</div> : null}
            </section>
        ) : null;

    const Popover = ({
        children,
        content,
    }: {
        children?: ReactNode;
        content?: ReactNode;
    }) => (
        <div data-testid="mock-popover">
            {children}
            {content ? <div>{content}</div> : null}
        </div>
    );

    const Tabs = ({
        activeKey,
        items,
    }: {
        activeKey?: string;
        items?: Array<{ key: string; label: ReactNode; children?: ReactNode }>;
    }) => {
        const activeItem = items?.find((item) => item.key === activeKey) ?? items?.[0];
        return (
            <section data-testid="mock-tabs">
                <nav>
                    {items?.map((item) => (
                        <span key={item.key}>{item.label}</span>
                    ))}
                </nav>
                <div>{activeItem?.children}</div>
            </section>
        );
    };

    const Table = <T extends Record<string, unknown>>({
        dataSource,
        columns,
        locale,
    }: {
        dataSource?: T[];
        columns?: Array<{
            key?: string;
            title?: ReactNode;
            dataIndex?: string | string[];
            render?: (value: unknown, record: T, index: number) => ReactNode;
        }>;
        locale?: { emptyText?: ReactNode };
    }) => {
        if (!dataSource || dataSource.length === 0) {
            return <div>{locale?.emptyText ?? null}</div>;
        }
        return (
            <table data-testid="mock-table">
                <thead>
                    <tr>
                        {columns?.map((column, index) => (
                            <th key={column.key ?? String(index)}>{column.title}</th>
                        ))}
                    </tr>
                </thead>
                <tbody>
                    {dataSource.map((record, rowIndex) => (
                        <tr key={String(record.id ?? record.ticket_id ?? rowIndex)}>
                            {columns?.map((column, columnIndex) => {
                                const rawValue = Array.isArray(column.dataIndex)
                                    ? column.dataIndex.reduce<unknown>(
                                        (value, key) =>
                                            value && typeof value === 'object'
                                                ? (value as Record<string, unknown>)[key]
                                                : undefined,
                                        record,
                                    )
                                    : typeof column.dataIndex === 'string'
                                        ? record[column.dataIndex]
                                        : undefined;
                                const content = column.render
                                    ? column.render(rawValue, record, rowIndex)
                                    : rawValue;
                                return <td key={column.key ?? String(columnIndex)}>{content as ReactNode}</td>;
                            })}
                        </tr>
                    ))}
                </tbody>
            </table>
        );
    };

    return {
        ...actual,
        Card,
        Descriptions,
        Drawer,
        Popover,
        Table,
        Tabs,
    };
});

vi.mock('@/features/approval-shared/summary', () => ({
    approvalEmptyValue: () => '—',
    approvalPrimaryAlert: () => null,
    approvalSummaryMeta: () => [],
    approvalSummarySections: () => ({ primary: [], secondary: [] }),
    approvalSummaryTitle: () => 'Request',
    buildApprovalBatchDisplayItems: () => [],
    buildApprovalChangeItems: () => [],
    buildApprovalOverviewItems: (ticket: Record<string, unknown>) => [
        { key: 'requester', label: 'Requester', children: ticket.requester ?? '—' },
        { key: 'approver', label: 'Approver', children: ticket.approver ?? '—' },
        { key: 'reason', label: 'Reason', children: ticket.reason ?? '—' },
    ],
    buildApprovalScopeItems: () => [],
    formatApprovalResourceShape: (cpu?: number, memory?: number, disk?: number) =>
        [cpu ? `${cpu} vCPU` : null, memory ? `${memory} Gi memory` : null, disk ? `${disk} Gi disk` : null]
            .filter(Boolean)
            .join(' · ') || undefined,
    formatApprovalRecordID: (id: string) => id,
}));

vi.mock('../hooks/useMyRequestsController', () => ({
    useMyRequestsController: () => ({
        data: {
            items: [
                {
                    id: 'ticket-pending-1',
                    operation_type: 'CREATE',
                    status: 'PENDING',
                    requester: 'alice',
                    reason: 'Need a VM',
                    created_at: new Date('2026-03-16T00:00:00Z').toISOString(),
                },
            ],
            pagination: { page: 1, per_page: 20, total: 1 },
        },
        isLoading: false,
        refetch: vi.fn(),
        view: 'in_progress',
        page: 1,
        pageSize: 20,
        historyStatus: 'SUCCESS',
        search: '',
        operationType: '',
        savedVmDraft: null,
        cancelMutation: { isPending: false, mutate: vi.fn() },
        activeBatchID: '',
        batchStatus: undefined,
        batchLoading: false,
        batchCanRetry: false,
        batchCanCancel: false,
        batchActionPending: false,
        messageContextHolder: null,
        setPage: vi.fn(),
        setPageSize: vi.fn(),
        changeView: vi.fn(),
        changeHistoryStatus: vi.fn(),
        applySearch: vi.fn(),
        applyOperationType: vi.fn(),
        clearListFilters: vi.fn(),
        discardSavedVmDraft: vi.fn(),
        prepareHistoryReuse: vi.fn(() => true),
        refreshBatch: vi.fn(),
        clearBatchTracking: vi.fn(),
        retryBatch: vi.fn(),
        cancelBatch: vi.fn(),
        ...state.overrides,
    }),
}));

import {
    buildRequestWorkbenchContextItems,
    buildRequestWorkbenchDetailItems,
    MyRequestsWorkbench,
} from './MyRequestsWorkbench';

describe('MyRequestsWorkbench', () => {
    beforeEach(() => {
        state.overrides = {};
        state.push.mockReset();
        state.searchParams = new URLSearchParams();
    });

    it('renders the in-progress requests table with the legacy page test id', () => {
        render(<MyRequestsWorkbench />);

        expect(screen.getByTestId('approvals-page')).toBeVisible();
        expect(screen.getAllByText('Need a VM').length).toBeGreaterThan(0);
        expect(screen.getByTestId('workbench-quick-search')).toBeVisible();
        expect(screen.getByTestId('approval-action-details-ticket-pending-1')).toBeVisible();
        expect(screen.getByTestId('approval-action-more-ticket-pending-1')).toBeVisible();
    });

    it('submits request quick search only after explicit submit', async () => {
        const user = userEvent.setup();
        const applySearch = vi.fn();
        state.overrides = {
            applySearch,
        };

        render(<MyRequestsWorkbench />);

        const input = screen.getByTestId('workbench-quick-search');
        await user.type(input, 'finance');
        expect(applySearch).not.toHaveBeenCalled();

        await user.keyboard('{Enter}');
        expect(applySearch).toHaveBeenCalledWith('finance');
    });

    it('shows history filters, opens request details, and reuses an approved request', async () => {
        const user = userEvent.setup();
        const prepareHistoryReuse = vi.fn(() => true);
        state.overrides = {
            view: 'history',
            prepareHistoryReuse,
            data: {
                items: [
                    {
                        id: 'ticket-approved-1',
                        operation_type: 'CREATE',
                        status: 'APPROVED',
                        requester: 'alice',
                        approver: 'bob',
                        reason: 'Need a VM',
                        created_at: new Date('2026-03-16T00:00:00Z').toISOString(),
                        updated_at: new Date('2026-03-16T01:00:00Z').toISOString(),
                        request_prefill: {
                            system_id: 'sys-1',
                            service_id: 'svc-1',
                            reason: 'Need a VM',
                            batch_count: 1,
                        },
                    },
                ],
                pagination: { page: 1, per_page: 20, total: 1 },
            },
        };

        render(<MyRequestsWorkbench />);

        expect(screen.getByTestId('approvals-status-filter')).toBeVisible();
        expect(screen.getByText('History')).toBeVisible();
        await user.click(screen.getByTestId('approval-action-details-ticket-approved-1'));

        expect(screen.getByText('Request Details')).toBeVisible();
        expect(screen.getByRole('button', { name: 'Open Request Context' })).toBeVisible();
        await user.click(screen.getByTestId('approval-action-reuse-ticket-approved-1'));

        expect(prepareHistoryReuse).toHaveBeenCalledWith(
            expect.objectContaining({ id: 'ticket-approved-1' })
        );
        expect(state.push).toHaveBeenCalledWith('/vms?request=create&draft=resume');
    });

    it('surfaces downstream execution failures after approval succeeds', async () => {
        const user = userEvent.setup();
        state.overrides = {
            view: 'history',
            data: {
                items: [
                    {
                        id: 'ticket-failed-1',
                        operation_type: 'MODIFY',
                        status: 'FAILED',
                        requester: 'alice',
                        approver: 'bob',
                        reason: 'Resize VM',
                        created_at: new Date('2026-03-16T00:00:00Z').toISOString(),
                        updated_at: new Date('2026-03-16T01:00:00Z').toISOString(),
                        provisioning: {
                            phase: 'Failed',
                            failure_message: 'CPU hotplug failed on the target node',
                        },
                        request_prefill: {
                            system_id: 'sys-1',
                            service_id: 'svc-1',
                            template_id: 'tpl-1',
                            instance_size_id: 'size-1',
                            namespace: 'team-prod',
                            reason: 'Resize VM',
                            batch_count: 1,
                        },
                    },
                ],
                pagination: { page: 1, per_page: 20, total: 1 },
            },
        };

        render(<MyRequestsWorkbench />);
        expect(screen.getByText('CPU hotplug failed on the target node')).toBeVisible();
        await user.click(screen.getByTestId('approval-action-details-ticket-failed-1'));

        expect(
            screen.getByText('Approval was accepted, but downstream execution failed.'),
        ).toBeVisible();
        expect(screen.getAllByText('CPU hotplug failed on the target node').length).toBeGreaterThan(0);
    });

    it('shows the drafts empty state and then a resumable local draft', () => {
        state.overrides = {
            view: 'drafts',
            data: undefined,
            savedVmDraft: null,
        };

        const { rerender } = render(<MyRequestsWorkbench />);

        expect(screen.getByText('No saved drafts yet')).toBeVisible();
        expect(screen.getByRole('button', { name: 'Open Virtual Machines' })).toBeVisible();

        state.overrides = {
            view: 'drafts',
            savedVmDraft: {
                version: 1,
                serviceLabel: 'Service A',
                templateLabel: 'Ubuntu 22.04',
                instanceSizeLabel: 'M4 Large',
                namespace: 'team-prod',
                batchCount: 2,
                wizardStep: 3,
                updatedAt: '2026-03-16T00:00:00Z',
            },
        };

        rerender(<MyRequestsWorkbench />);

        expect(screen.getByText('Saved VM request draft')).toBeVisible();
        expect(screen.getByRole('button', { name: 'Resume Draft' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Discard Draft' })).toBeVisible();
        expect(screen.getByText('Service A')).toBeVisible();
    });

    it('renders active batch tracking inside the batch jobs tab', () => {
        state.searchParams = new URLSearchParams('tab=batch_jobs');
        state.overrides = {
            view: 'batch_jobs',
            activeBatchID: 'batch-1',
            batchStatus: {
                batch_id: 'batch-1',
                operation: 'CREATE',
                status: 'IN_PROGRESS',
                child_count: 2,
                success_count: 1,
                failed_count: 0,
                pending_count: 1,
                children: [
                    {
                        ticket_id: 'ticket-1',
                        resource_name: 'vm-a',
                        status: 'PENDING',
                        attempt_count: 0,
                        last_error: '',
                    },
                ],
            },
        };

        render(<MyRequestsWorkbench />);

        expect(screen.getByText('Current Batch Job')).toBeVisible();
        expect(screen.getByText('Create · 2 VM requests')).toBeVisible();
        expect(screen.getAllByText('vm-a').length).toBeGreaterThan(0);
        expect(screen.getByRole('button', { name: 'Retry Failed' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Cancel Pending' })).toBeVisible();
    });

    it('builds request detail items from a ticket snapshot', () => {
        const fakeT = ((key: string) => key) as unknown as TFunction;
        const ticket = {
            id: 'ticket-approved-1',
            operation_type: 'CREATE',
            status: 'APPROVED',
            requester: 'alice',
            approver: 'bob',
            reason: 'Need a VM',
            created_at: new Date('2026-03-16T00:00:00Z').toISOString(),
            updated_at: new Date('2026-03-16T01:00:00Z').toISOString(),
        };

        const items = buildRequestWorkbenchDetailItems(ticket as never, fakeT);
        expect(items.map((item) => item.key)).toEqual([
            'requester',
            'approver',
            'reason',
            'created',
            'updated',
        ]);
    });

    it('builds reusable request context items from request prefill', () => {
        const fakeT = ((key: string) => key) as unknown as TFunction;
        const ticket = {
            summary: {
                system_name: 'Payments',
                service_name: 'Billing',
                template_name: 'Ubuntu 24.04',
                instance_size_name: 'M4 Large',
                namespace: 'team-prod',
            },
            request_prefill: {
                system_id: 'sys-1',
                service_id: 'svc-1',
                template_id: 'tpl-1',
                instance_size_id: 'size-1',
                namespace: 'team-prod',
                batch_count: 2,
            },
        };

        const items = buildRequestWorkbenchContextItems(ticket as never, fakeT);
        expect(items).toEqual([
            { key: 'system', label: 'workbench.details.system', value: 'Payments', isIdentifier: false },
            { key: 'service', label: 'workbench.details.service', value: 'Billing', isIdentifier: false },
            { key: 'template', label: 'workbench.drafts.template', value: 'Ubuntu 24.04', isIdentifier: false },
            { key: 'size', label: 'workbench.drafts.size', value: 'M4 Large', isIdentifier: false },
            { key: 'namespace', label: 'workbench.drafts.namespace', value: 'team-prod' },
            { key: 'batch_count', label: 'workbench.drafts.batch_count', value: 2 },
        ]);
    });
});
