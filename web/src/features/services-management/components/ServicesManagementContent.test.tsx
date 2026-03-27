import { Form } from 'antd';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const pushMock = vi.fn();
const useApiGetMock = vi.fn();
const openCreateModalMock = vi.fn();

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        push: pushMock,
    }),
    useSearchParams: () => new URLSearchParams(window.location.search),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (
            key: string,
            options?: string | { defaultValue?: string; [key: string]: unknown },
        ) => {
            const labels: Record<string, string> = {
                'nav.services': 'Services',
                'services.subtitle': 'Manage services within systems',
                'services.select_system': 'Select system',
                'services.all_systems': 'All systems',
                'services.system_column': 'System',
                'services.context_title': 'Service Context',
                'services.context_system': 'System',
                'services.context_next_index': 'Next Instance Index',
                'services.context_summary': 'Workspace Summary',
                'services.context_summary_value': '1 visible VM(s), 1 recent request(s)',
                'services.related_vms_title': 'Visible Virtual Machines',
                'services.related_vms_empty': 'No visible virtual machines were found for this service',
                'services.related_requests_title': 'My Recent Requests',
                'services.related_requests_empty': 'No recent requests were found for this service',
                'services.open_vm_workspace': 'Open VM Workspace',
                'services.open_vm_detail': 'Open VM',
                'services.open_my_requests': 'Open My Requests',
                'services.empty_description': 'No description provided',
                'table.name': 'Name',
                'table.description': 'Description',
                'table.created_at': 'Created',
                'table.status': 'Status',
                'table.actions': 'Actions',
                'services.instance_index': 'Next Instance Index',
                'services.next_instance_index': 'Next Instance Index',
                'button.refresh': 'Refresh',
                'button.create': 'Create',
                'button.close': 'Close',
                'services.request_vm': 'Request VM',
                'table.total': 'Total',
                'message.confirm_delete': 'Delete?',
                'button.confirm': 'Confirm',
                'button.cancel': 'Cancel',
                'field.namespace': 'Namespace',
                ticket_id: 'Ticket ID',
                request_summary: 'Request Summary',
                request_batch_count: '{{count}} requests',
                operation_type: 'Operation',
                'op_type.CREATE': 'Create',
                'op_type.DELETE': 'Delete',
                reason: 'Reason',
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

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
}));

vi.mock('@/components/auth/PermissionGuard', () => ({
    PermissionGuard: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock('../hooks/useServicesManagementController', () => ({
    useServicesManagementController: () => {
        const [form] = Form.useForm();
        const [editForm] = Form.useForm();

        return {
            messageContextHolder: null,
            createOpen: false,
            editOpen: false,
            editingService: null,
            activeSystemId: 'sys-1',
            page: 1,
            pageSize: 20,
            setPage: vi.fn(),
            setPageSize: vi.fn(),
            form,
            editForm,
            systemsData: {
                items: [{ id: 'sys-1', name: 'System A' }],
            },
            servicesData: {
                items: [
                    {
                        id: 'svc-1',
                        system_id: 'sys-1',
                        system_name: 'System A',
                        name: 'Service A',
                        description: 'frontend',
                        next_instance_index: 3,
                        created_at: '2026-03-16T00:00:00Z',
                    },
                ],
                pagination: { page: 1, per_page: 20, total: 1 },
            },
            isLoading: false,
            refetch: vi.fn(),
            changeSystem: vi.fn(),
            openCreateModal: openCreateModalMock,
            closeCreateModal: vi.fn(),
            openEditModal: vi.fn(),
            closeEditModal: vi.fn(),
            submitCreate: vi.fn(),
            submitEdit: vi.fn(),
            submitDelete: vi.fn(),
            createPending: false,
            updatePending: false,
            deletePending: false,
        };
    },
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

import { ServicesManagementContent } from './ServicesManagementContent';

describe('ServicesManagementContent', () => {
    beforeEach(() => {
        pushMock.mockReset();
        openCreateModalMock.mockReset();
        window.history.replaceState({}, '', '/services');
        useApiGetMock.mockReturnValue({
            data: {
                service: {
                    id: 'svc-1',
                    system_id: 'sys-1',
                    system_name: 'System A',
                    name: 'Service A',
                    description: 'frontend',
                    next_instance_index: 3,
                    created_at: '2026-03-16T00:00:00Z',
                },
                summary: {
                    visible_vm_count: 1,
                    recent_request_count: 1,
                },
                visible_vms: [
                    {
                        id: 'vm-1',
                        service_id: 'svc-1',
                        name: 'vm-a',
                        namespace: 'team-prod',
                        status: 'RUNNING',
                        created_at: '2026-03-16T00:00:00Z',
                    },
                ],
                recent_requests: [
                    {
                        id: 'ticket-1',
                        operation_type: 'CREATE',
                        status: 'APPROVED',
                        requester: 'alice',
                        reason: 'Need a VM',
                        created_at: '2026-03-16T00:00:00Z',
                        request_prefill: {
                            service_id: 'svc-1',
                        },
                    },
                ],
            },
            isLoading: false,
        });
    });

    it('auto-opens the service create flow with the selected system from onboarding intent', () => {
        window.history.replaceState({}, '', '/services?intent=create-service&system_id=sys-1');

        render(<ServicesManagementContent />);

        expect(openCreateModalMock).toHaveBeenCalledWith('sys-1');
    });

    it('offers a visible request-vm action that carries service context into the vm page', () => {
        render(<ServicesManagementContent />);

        fireEvent.click(screen.getByTestId('service-action-request-vm-svc-1'));

        expect(pushMock).toHaveBeenCalledWith('/vms?request=create&system_id=sys-1&service_id=svc-1');
    });

    it('renders the owning system as a clickable link without exposing the raw system id', () => {
        render(<ServicesManagementContent />);

        fireEvent.click(screen.getByTestId('service-action-open-system-svc-1'));

        expect(pushMock).toHaveBeenCalledWith('/systems?detail_system_id=sys-1');
        expect(screen.queryByText('sys-1')).not.toBeInTheDocument();
    }, 15000);

    it('shows a service context modal with related VMs and requests', async () => {
        render(<ServicesManagementContent />);

        fireEvent.click(screen.getByTestId('service-action-detail-svc-1'));

        const detailModalTitle = await screen.findByText('Service Context');
        const detailModal = detailModalTitle.closest('.ant-modal') as HTMLElement | null;

        expect(detailModal).not.toBeNull();

        const modalQueries = within(detailModal!);
        expect(modalQueries.getByText('Service Context')).toBeInTheDocument();
        expect(modalQueries.getByText('Visible Virtual Machines')).toBeInTheDocument();
        expect(modalQueries.getByText('My Recent Requests')).toBeInTheDocument();
        expect(modalQueries.getByText('vm-a')).toBeInTheDocument();
        expect(modalQueries.getByText('Create')).toBeInTheDocument();
        expect(modalQueries.getByText('Need a VM')).toBeInTheDocument();
        expect(modalQueries.getByText('Ticket ID: ticket-1')).toBeInTheDocument();
        expect(
            modalQueries.getByRole('button', { name: 'Request VM' }),
        ).toBeInTheDocument();
        expect(
            modalQueries.getAllByRole('button', { name: 'Open My Requests' }).length,
        ).toBeGreaterThan(0);
        expect(
            modalQueries.getByRole('button', { name: 'Open VM Workspace' }),
        ).toBeInTheDocument();
    }, 20000);

    it('opens the service detail modal from deep link context', async () => {
        window.history.replaceState({}, '', '/services?system_id=sys-1&detail_service_id=svc-1');

        render(<ServicesManagementContent />);

        const detailModalTitle = await screen.findByText('Service Context');
        const detailModal = detailModalTitle.closest('.ant-modal') as HTMLElement | null;
        expect(detailModal).not.toBeNull();

        const modalQueries = within(detailModal!);
        expect(modalQueries.getByText('Service A')).toBeInTheDocument();
        expect(modalQueries.getByText('System A')).toBeInTheDocument();
        expect(modalQueries.getByText('vm-a')).toBeInTheDocument();
    }, 20000);

    it('keeps system and service context when opening the VM workspace from service detail', async () => {
        render(<ServicesManagementContent />);

        fireEvent.click(screen.getByTestId('service-action-detail-svc-1'));

        const detailModalTitle = await screen.findByText('Service Context');
        const detailModal = detailModalTitle.closest('.ant-modal') as HTMLElement | null;

        expect(detailModal).not.toBeNull();

        const openWorkspaceButton = within(detailModal!).getByRole('button', {
            name: 'Open VM Workspace',
        });

        fireEvent.click(openWorkspaceButton);

        await waitFor(() => {
            expect(pushMock).toHaveBeenCalledWith('/vms?system_id=sys-1&service_id=svc-1');
        });
    }, 20000);
});
