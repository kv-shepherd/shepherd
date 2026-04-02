import { Form } from 'antd';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const useSetupGuideMock = vi.fn();
const useScopedVMRequestLauncherMock = vi.fn();
const pushMock = vi.fn();
const useApiGetMock = vi.fn();
const openWizardMock = vi.fn();
const changeFiltersMock = vi.fn();
const resetFiltersMock = vi.fn();

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
            batchRateLimited: false,
            modifySubmitPending: false,
            openBatchModifyModal: vi.fn(),
            submitBatchPowerSelected: vi.fn(),
            submitBatchDeleteSelected: vi.fn(),
            batchRetryAfterSeconds: 0,
            vmData: { items: [], pagination: { total: 0 } },
            isLoading: false,
            page: 1,
            pageSize: 20,
            setPage: vi.fn(),
            setPageSize: vi.fn(),
            startVM: vi.fn(),
            stopVM: vi.fn(),
            restartVM: vi.fn(),
            openDeleteModal: vi.fn(),
            openModifyModal: vi.fn(),
            setSelectedVMIDs: vi.fn(),
            activeBatchID: '',
            batchStatus: null,
            batchLoading: false,
            refreshBatch: vi.fn(),
            clearBatchTracking: vi.fn(),
            lastBatchActionFeedback: null,
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
});
