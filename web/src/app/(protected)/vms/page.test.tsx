import { Form } from 'antd';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const controllerState = vi.hoisted(() => ({
    activeBatchID: '',
    activeBatchKind: '',
    batchRateLimited: false,
    batchRateLimitContactAdmin: false,
    batchRetryAfterSeconds: 0,
    canRetryActiveBatch: false,
    canCancelActiveBatch: false,
    lastBatchActionFeedback: null as null | {
        action: 'retry' | 'cancel';
        affectedCount: number;
        affectedTicketIDs: string[];
    },
    restartReconciliationNotice: null as null | {
        eventId: string;
        reconciliationPath: string;
    },
    batchStatus: null as null | {
        status: string;
        success_count?: number;
        failed_count?: number;
        pending_count?: number;
    },
}));

const useSetupGuideMock = vi.fn();
const useScopedVMRequestLauncherMock = vi.fn();
const pushMock = vi.fn();
const useApiGetMock = vi.fn();
const openWizardMock = vi.fn();
const changeFiltersMock = vi.fn();
const resetFiltersMock = vi.fn();
const retryBatchMock = vi.fn();
const cancelBatchMock = vi.fn();
const dismissRestartReconciliationMock = vi.fn();

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (
            key: string,
            options?: string | { defaultValue?: string; [key: string]: unknown },
        ) => {
            const labels: Record<string, string> = {
                title: 'Virtual Machines',
                subtitle: 'Manage virtual machine lifecycle',
                create_request: 'Request VM',
                'context.title': 'Service workspace context',
                'context.description':
                    'Requests from this page will be prefilled for Billing API in Payments. The VM list still shows all VMs you can access.',
                'context.system': 'System',
                'context.service': 'Service',
                'context.summary': 'Workspace Summary',
                'context.summary_value': '2 visible VM(s), 1 recent request(s)',
                'context.open_service': 'Open Service',
                'context.clear': 'Clear Context',
                'common:button.refresh': 'Refresh',
                'common:button.search': 'Search',
                'common:button.clear_filters': 'Clear filters',
                'common:search.advanced': 'Advanced search',
                'common:search.hide_advanced': 'Hide advanced search',
                'common:environment.prod': 'Production',
                'common:environment.test': 'Test',
                search_placeholder: 'Search VMs',
                search_help: 'Search help',
                advanced_search_title: 'Advanced search',
                advanced_search_help: 'Advanced search help',
                'search.status': 'Status',
                'search.namespace': 'Namespace',
                'search.cluster': 'Cluster',
                'search.system': 'System',
                'search.service': 'Service',
                'search.operating_system': 'Operating system',
                'search.ip_address': 'IP address',
                'batch.current_batch': 'Current Batch',
                'batch.request_live_status_summary': 'Current batch request status PENDING_APPROVAL. Open My Requests to follow approval and downstream handling.',
                'batch.job_live_status_summary': 'Current batch status IN_PROGRESS, success 2, failed 0, pending 1',
                'batch.open_requests': 'Open My Requests',
                'batch.open_workbench': 'Open Batch Jobs',
                'batch.clear': 'Clear',
                'batch.retry_failed': 'Retry Failed',
                'batch.cancel_pending': 'Cancel Pending',
                'batch.retry_feedback': 'Retry affected {{count}}: {{ids}}',
                'batch.cancel_feedback': 'Cancel affected {{count}}: {{ids}}',
                'batch.affected_ids_none': 'none returned',
                'restart_reconciliation.title': 'Operator action required',
                'restart_reconciliation.readonly_description': 'Follow the operations runbook; the UI cannot release this fence.',
                'restart_reconciliation.event_id': 'Event ID',
                'restart_reconciliation.path': 'Reconciliation path',
                'restart_reconciliation.dismiss': 'Dismiss',
                'batch.rate_limited_wait': 'Retry in {{seconds}}s',
                'batch.rate_limited_contact_admin': 'Retry in {{seconds}}s or contact an administrator',
            };
            const fallback =
                typeof options === 'string' ? options : options?.defaultValue;
            let message = labels[key] ?? fallback ?? key;
            if (options && typeof options === 'object') {
                for (const [name, value] of Object.entries(options)) {
                    if (name === 'defaultValue') {
                        continue;
                    }
                    message = message.replaceAll(`{{${name}}}`, String(value));
                }
            }
            return message;
        },
    }),
}));

vi.mock('antd', async (importOriginal) => {
    const actual = await importOriginal<typeof import('antd')>();

    return {
        ...actual,
        Modal: ({
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
                <section className="ant-modal">
                    {title ? <header>{title}</header> : null}
                    <div>{children}</div>
                    {footer ? <footer>{footer}</footer> : null}
                </section>
            ) : null,
    };
});

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        push: pushMock,
    }),
    useSearchParams: () => new URLSearchParams(window.location.search),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/components/auth/PermissionGuard', () => ({
    PermissionGuard: ({ children }: { children: React.ReactNode }) => children,
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

vi.mock('@/features/setup-guide/components/SetupGuideCard', () => ({
    SetupGuideCard: ({ variant }: { variant: string }) => <div>{`setup-guide-${variant}`}</div>,
}));

vi.mock('@/features/setup-guide/hooks/useSetupGuide', () => ({
    useSetupGuide: (...args: unknown[]) => useSetupGuideMock(...args),
}));

vi.mock('@/features/vm-management/components/VMSavedDraftBanner', () => ({
    VMSavedDraftBanner: () => <div>saved-draft-banner</div>,
}));

vi.mock('@/features/vm-management/components/VMListTable', () => ({
    VMListTable: () => <div>vm-list-table</div>,
}));

vi.mock('@/features/vm-management/components/VMRequestWizard', () => ({
    VMRequestWizard: () => null,
}));

vi.mock('@/features/vm-management/hooks/useScopedVMRequestLauncher', () => ({
    useScopedVMRequestLauncher: (...args: unknown[]) => useScopedVMRequestLauncherMock(...args),
}));

vi.mock('@/features/vm-management/hooks/useVMManagementController', () => ({
    useVMManagementController: () => {
        const [form] = Form.useForm();
        const [modifyForm] = Form.useForm();

        return {
            messageContextHolder: null,
            savedDraft: null,
            wizardOpen: false,
            refetch: vi.fn(),
            openWizard: openWizardMock,
            openSimilarRequest: vi.fn(),
            resumeDraft: vi.fn(),
            discardDraft: vi.fn(),
            selectedVMIDs: [],
            batchSubmitPending: false,
            batchRateLimited: controllerState.batchRateLimited,
            batchRateLimitContactAdmin: controllerState.batchRateLimitContactAdmin,
            modifySubmitPending: false,
            openBatchModifyModal: vi.fn(),
            submitBatchPowerSelected: vi.fn(),
            submitBatchDeleteSelected: vi.fn(),
            batchRetryAfterSeconds: controllerState.batchRetryAfterSeconds,
            canRetryActiveBatch: controllerState.canRetryActiveBatch,
            canCancelActiveBatch: controllerState.canCancelActiveBatch,
            lastBatchActionFeedback: controllerState.lastBatchActionFeedback,
            batchActionPending: false,
            retryBatch: retryBatchMock,
            cancelBatch: cancelBatchMock,
            vmData: { items: [], pagination: { total: 0 } },
            isLoading: false,
            page: 1,
            pageSize: 20,
            setPage: vi.fn(),
            setPageSize: vi.fn(),
            startVM: vi.fn(),
            stopVM: vi.fn(),
            restartVM: vi.fn(),
            restartReconciliationNotice: controllerState.restartReconciliationNotice,
            dismissRestartReconciliation: dismissRestartReconciliationMock,
            openDeleteModal: vi.fn(),
            openModifyModal: vi.fn(),
            setSelectedVMIDs: vi.fn(),
            activeBatchID: controllerState.activeBatchID,
            activeBatchKind: controllerState.activeBatchKind,
            batchStatus: controllerState.batchStatus,
            batchLoading: false,
            refreshBatch: vi.fn(),
            clearBatchTracking: vi.fn(),
            wizardStep: 0,
            setWizardStep: vi.fn(),
            requestMode: 'guided',
            setRequestMode: vi.fn(),
            filters: {
                search: '',
                namespace: '',
                status: '',
                clusterId: '',
                systemId: '',
                serviceId: '',
                osName: '',
                ipAddress: '',
            },
            changeFilters: changeFiltersMock,
            resetFilters: resetFiltersMock,
            form,
            wizardSteps: [],
            selectedSystemId: '',
            onSystemChange: vi.fn(),
            systemsData: undefined,
            servicesData: undefined,
            templatesData: undefined,
            sizesData: undefined,
            selectedTemplate: undefined,
            selectedSize: undefined,
            placementHint: undefined,
            placementHintLoading: false,
            serviceIdValue: undefined,
            namespaceValue: undefined,
            namespaceOptions: [],
            reasonValue: undefined,
            batchCountValue: 1,
            createVMRequest: { isPending: false },
            vmFilterOptions: {
                statuses: [{ value: 'RUNNING', label: 'RUNNING' }],
                namespaces: [{ value: 'prod-apps', label: 'prod-apps', group: 'prod' }],
                clusters: [{ value: 'cluster-a', label: 'Cluster A', group: 'prod' }],
                systems: [{ value: 'sys-1', label: 'Payments' }],
                services: [{ value: 'svc-1', label: 'Payments / Billing API', group: 'Payments' }],
                operating_systems: [{ value: 'Ubuntu 24.04', label: 'Ubuntu 24.04' }],
                ip_addresses: [{ value: '10.6.194.9', label: '10.6.194.9', group: 'test' }],
            },
            vmFilterOptionsLoading: false,
            goToNextWizardStep: vi.fn(),
            submitWizard: vi.fn(),
            closeWizard: vi.fn(),
            modifyScope: 'single',
            modifyTargetVM: null,
            modifyOpen: false,
            submitModify: vi.fn(),
            closeModifyModal: vi.fn(),
            modifySubmitDisabled: false,
            modifyForm,
            modifyContext: null,
            modifyContextLoading: false,
            deleteOpen: false,
            submitDelete: vi.fn(),
            closeDeleteModal: vi.fn(),
            deletePending: false,
            deletingVM: null,
            deleteConfirmName: '',
            setDeleteConfirmName: vi.fn(),
        };
    },
}));

import VMsPage from './page';

describe('VMsPage', () => {
    beforeEach(() => {
        pushMock.mockReset();
        openWizardMock.mockReset();
        changeFiltersMock.mockReset();
        resetFiltersMock.mockReset();
        retryBatchMock.mockReset();
        cancelBatchMock.mockReset();
        dismissRestartReconciliationMock.mockReset();
        controllerState.activeBatchID = '';
        controllerState.activeBatchKind = '';
        controllerState.batchRateLimited = false;
        controllerState.batchRateLimitContactAdmin = false;
        controllerState.batchRetryAfterSeconds = 0;
        controllerState.canRetryActiveBatch = false;
        controllerState.canCancelActiveBatch = false;
        controllerState.lastBatchActionFeedback = null;
        controllerState.restartReconciliationNotice = null;
        controllerState.batchStatus = null;
        useApiGetMock.mockReturnValue({
            data: {
                service: {
                    id: 'svc-1',
                    system_id: 'sys-1',
                    system_name: 'Payments',
                    name: 'Billing API',
                },
                summary: {
                    visible_vm_count: 2,
                    recent_request_count: 1,
                },
            },
            isLoading: false,
        });
        window.history.replaceState({}, '', '/vms');
    });

    it('shows setup guidance and disables create when request prerequisites are missing', () => {
        useSetupGuideMock.mockReturnValue({
            vmRequestReady: false,
        });
        useScopedVMRequestLauncherMock.mockReset();

        render(<VMsPage />);

        expect(screen.getByText('setup-guide-vm')).toBeVisible();
        expect(screen.getByRole('button', { name: /Request VM/ })).toBeDisabled();
        expect(useScopedVMRequestLauncherMock).toHaveBeenCalledWith(
            expect.objectContaining({ canLaunchRequest: false }),
        );
    });

    it('shows scoped workspace context and prefills request actions for the selected service', async () => {
        const user = userEvent.setup();
        useSetupGuideMock.mockReturnValue({
            vmRequestReady: true,
        });
        window.history.replaceState({}, '', '/vms?system_id=sys-1&service_id=svc-1');

        render(<VMsPage />);

        expect(screen.getByText('Service workspace context')).toBeVisible();
        expect(screen.getByText('Payments')).toBeVisible();
        expect(screen.getByText('Billing API')).toBeVisible();
        expect(screen.getByText('2 visible VM(s), 1 recent request(s)')).toBeVisible();

        await user.click(screen.getAllByRole('button', { name: 'Request VM' })[0]);
        expect(openWizardMock).toHaveBeenCalledWith({
            systemId: 'sys-1',
            serviceId: 'svc-1',
        });

        await user.click(screen.getByRole('button', { name: 'Open Service' }));
        expect(pushMock).toHaveBeenCalledWith('/services?system_id=sys-1&detail_service_id=svc-1');
    });

    it('shows advanced search controls for exact VM filters', async () => {
        const user = userEvent.setup();
        useSetupGuideMock.mockReturnValue({
            vmRequestReady: true,
        });

        render(<VMsPage />);

        expect(screen.getByTestId('vms-quick-search')).toBeVisible();
        await user.click(screen.getByTestId('vms-advanced-search-toggle'));
        expect(screen.getByTestId('vms-filter-cluster')).toBeVisible();
        expect(screen.getByTestId('vms-filter-namespace')).toBeVisible();
        expect(screen.getByTestId('vms-filter-service')).toBeVisible();
        expect(screen.getByTestId('vms-advanced-search-submit')).toBeVisible();
    });

    it('keeps administrator guidance visible throughout a server-directed batch cooldown', () => {
        useSetupGuideMock.mockReturnValue({
            vmRequestReady: true,
        });
        controllerState.batchRateLimited = true;
        controllerState.batchRateLimitContactAdmin = true;
        controllerState.batchRetryAfterSeconds = 9;

        render(<VMsPage />);

        expect(screen.getByText('Retry in 9s or contact an administrator')).toBeVisible();
    });

    it('routes request-style current batch tracking to My Requests', async () => {
        const user = userEvent.setup();
        useSetupGuideMock.mockReturnValue({
            vmRequestReady: true,
        });
        controllerState.activeBatchID = 'batch-req-1';
        controllerState.activeBatchKind = 'request';
        controllerState.batchStatus = {
            status: 'PENDING_APPROVAL',
            success_count: 0,
            failed_count: 0,
            pending_count: 2,
        };

        render(<VMsPage />);

        expect(screen.getByText('Current Batch')).toBeVisible();
        expect(screen.getAllByText(/Open My Requests to follow approval/).length).toBeGreaterThan(0);

        await user.click(screen.getByRole('button', { name: 'Open My Requests' }));
        expect(pushMock).toHaveBeenCalledWith('/tickets?tab=in_progress');
    });

    it('routes operation-style current batch tracking to the batch detail page', async () => {
        const user = userEvent.setup();
        useSetupGuideMock.mockReturnValue({
            vmRequestReady: true,
        });
        controllerState.activeBatchID = 'batch-job-1';
        controllerState.activeBatchKind = 'job';
        controllerState.batchStatus = {
            status: 'IN_PROGRESS',
            success_count: 2,
            failed_count: 0,
            pending_count: 1,
        };

        render(<VMsPage />);

        await user.click(screen.getByRole('button', { name: 'Open Batch Jobs' }));
        expect(pushMock).toHaveBeenCalledWith('/vms/batch/batch-job-1');
    });

    it('consumes controller retry/cancel capability and affected ticket feedback', async () => {
        const user = userEvent.setup();
        useSetupGuideMock.mockReturnValue({ vmRequestReady: true });
        controllerState.activeBatchID = 'batch-job-1';
        controllerState.activeBatchKind = 'job';
        controllerState.batchStatus = {
            status: 'IN_PROGRESS',
            success_count: 0,
            failed_count: 1,
            pending_count: 1,
        };
        controllerState.canRetryActiveBatch = true;
        controllerState.canCancelActiveBatch = true;
        controllerState.lastBatchActionFeedback = {
            action: 'retry',
            affectedCount: 1,
            affectedTicketIDs: ['ticket-failed-1'],
        };

        render(<VMsPage />);

        expect(screen.getByTestId('active-batch-action-feedback')).toHaveTextContent(
            'Retry affected 1: ticket-failed-1',
        );
        await user.click(screen.getByTestId('active-batch-retry'));
        await user.click(screen.getByTestId('active-batch-cancel'));
        expect(retryBatchMock).toHaveBeenCalledOnce();
        expect(cancelBatchMock).toHaveBeenCalledOnce();
    });

    it('shows ambiguous restart metadata as read-only runbook guidance', async () => {
        const user = userEvent.setup();
        useSetupGuideMock.mockReturnValue({ vmRequestReady: true });
        controllerState.restartReconciliationNotice = {
            eventId: 'event-restart-1',
            reconciliationPath: 'operator-runbook:ambiguous-vm-restart',
        };

        render(<VMsPage />);

        const alert = screen.getByTestId('restart-reconciliation-alert');
        expect(alert).toHaveTextContent('event-restart-1');
        expect(alert).toHaveTextContent(
            'operator-runbook:ambiguous-vm-restart',
        );
        expect(alert).toHaveTextContent('the UI cannot release this fence');
        expect(screen.queryByTestId('restart-reconciliation-submit')).not.toBeInTheDocument();
        await user.click(screen.getByRole('button', { name: 'Dismiss' }));
        expect(dismissRestartReconciliationMock).toHaveBeenCalledOnce();
    });
});
