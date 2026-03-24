import { fireEvent, render, screen } from '@testing-library/react';
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
                'common:button.close': 'Close',
                'common:table.total': 'Total',
                'status.PENDING': 'Pending',
                'status.APPROVED': 'Approved',
                'status.REJECTED': 'Rejected',
                'status.CANCELLED': 'Cancelled',
                'status.SUCCESS': 'Success',
                'status.FAILED': 'Failed',
                'vm:batch.clear': 'Clear',
                'vm:batch.status': 'Status',
                'vm:batch.operation': 'Operation',
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
        discardSavedVmDraft: vi.fn(),
        prepareHistoryReuse: vi.fn(() => true),
        refreshBatch: vi.fn(),
        clearBatchTracking: vi.fn(),
        retryBatch: vi.fn(),
        cancelBatch: vi.fn(),
        ...state.overrides,
    }),
}));

import { MyRequestsWorkbench } from './MyRequestsWorkbench';

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
        expect(screen.getByTestId('approval-action-cancel-ticket-pending-1')).toBeVisible();
        expect(screen.getByTestId('approval-action-details-ticket-pending-1')).toBeVisible();
    });

    it('shows the history status segmented control in history view', () => {
        state.overrides = {
            view: 'history',
            data: {
                items: [
                    {
                        id: 'ticket-success-1',
                        operation_type: 'CREATE',
                        status: 'SUCCESS',
                        requester: 'alice',
                        reason: 'Need a VM',
                        created_at: new Date('2026-03-16T00:00:00Z').toISOString(),
                        request_prefill: {
                            system_id: 'sys-1',
                            service_id: 'svc-1',
                            template_id: 'tpl-1',
                            instance_size_id: 'size-1',
                            namespace: 'team-prod',
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
        expect(screen.getByRole('button', { name: 'Reuse Request' })).toBeVisible();
    }, 10000);

    it('opens a request details drawer with reusable request context', () => {
        state.overrides = {
            view: 'history',
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
                        summary: {
                            system_name: 'System A',
                            service_name: 'Service A',
                            namespace: 'team-prod',
                            cluster_name: 'Prod Cluster',
                            template_name: 'Ubuntu 22.04',
                            instance_size_name: 'M4 Large',
                            batch_count: 2,
                        },
                        request_prefill: {
                            system_id: 'sys-1',
                            service_id: 'svc-1',
                            template_id: 'tpl-1',
                            instance_size_id: 'size-1',
                            namespace: 'team-prod',
                            reason: 'Need a VM',
                            batch_count: 2,
                        },
                    },
                ],
                pagination: { page: 1, per_page: 20, total: 1 },
            },
        };

        render(<MyRequestsWorkbench />);
        fireEvent.click(screen.getByTestId('approval-action-details-ticket-approved-1'));

        expect(screen.getByText('Request Details')).toBeVisible();
        expect(screen.getByText('Request Summary')).toBeVisible();
        expect(screen.getByText('Resource Context')).toBeVisible();
        expect(screen.getByText('Recovered Request Context')).toBeVisible();
        expect(screen.getAllByText('System A').length).toBeGreaterThan(0);
        expect(screen.getAllByText('Service A').length).toBeGreaterThan(0);
        expect(screen.getByText('Prod Cluster')).toBeVisible();
        expect(screen.getAllByText('Ubuntu 22.04').length).toBeGreaterThan(0);
        expect(screen.getByText('sys-1')).toBeVisible();
        expect(screen.getByText('svc-1')).toBeVisible();
        expect(screen.getByRole('button', { name: 'Open Request Context' })).toBeVisible();
        expect(screen.getAllByRole('button', { name: 'Reuse Request' }).length).toBeGreaterThan(0);
    }, 10000);

    it('shows a guided empty state for drafts', () => {
        state.overrides = {
            view: 'drafts',
            data: undefined,
            savedVmDraft: null,
        };

        render(<MyRequestsWorkbench />);

        expect(screen.getByText('No saved drafts yet')).toBeVisible();
        expect(screen.getByRole('button', { name: 'Open Virtual Machines' })).toBeVisible();
    });

    it('shows a resumable local draft in the drafts tab', () => {
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

        render(<MyRequestsWorkbench />);

        expect(screen.getByText('Saved VM request draft')).toBeVisible();
        expect(screen.getByRole('button', { name: 'Resume Draft' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Discard Draft' })).toBeVisible();
        expect(screen.getByText('Service A')).toBeVisible();
    });

    it('routes to the vm request flow after preparing a history reuse draft', () => {
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
                        reason: 'Need a VM',
                        created_at: new Date('2026-03-16T00:00:00Z').toISOString(),
                        request_prefill: {
                            system_id: 'sys-1',
                            service_id: 'svc-1',
                            template_id: 'tpl-1',
                            instance_size_id: 'size-1',
                            namespace: 'team-prod',
                            reason: 'Need a VM',
                            batch_count: 1,
                        },
                    },
                ],
                pagination: { page: 1, per_page: 20, total: 1 },
            },
        };

        render(<MyRequestsWorkbench />);
        fireEvent.click(screen.getByRole('button', { name: 'Reuse Request' }));

        expect(prepareHistoryReuse).toHaveBeenCalledWith(
            expect.objectContaining({ id: 'ticket-approved-1' })
        );
        expect(state.push).toHaveBeenCalledWith('/vms?request=create&draft=resume');
    }, 15000);

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
        expect(screen.getAllByText('batch-1').length).toBeGreaterThan(0);
        expect(screen.getByRole('button', { name: 'Retry Failed' })).toBeVisible();
        expect(screen.getByRole('button', { name: 'Cancel Pending' })).toBeVisible();
    }, 15000);
});
