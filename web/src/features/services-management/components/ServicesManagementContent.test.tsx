import { Form } from 'antd';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const pushMock = vi.fn();
const useApiGetMock = vi.fn();
const openCreateModalMock = vi.fn();
const applyFiltersMock = vi.fn();
const clearFiltersMock = vi.fn();

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
                'button.filter': 'Filters',
                'button.hide_filters': 'Hide filters',
                'button.search': 'Search',
                'common:button.edit': 'Edit',
                'common:button.delete': 'Delete',
                'common:button.refresh': 'Refresh',
                'common:button.create': 'Create',
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

vi.mock('antd', async (importOriginal) => {
    const actual = await importOriginal<typeof import('antd')>();

    return {
        ...actual,
        Card: ({
            title,
            extra,
            children,
        }: {
            title?: ReactNode;
            extra?: ReactNode;
            children?: ReactNode;
        }) => (
            <section data-testid="antd-card">
                {title ? <header>{title}</header> : null}
                {extra ? <div>{extra}</div> : null}
                <div>{children}</div>
            </section>
        ),
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
        Table: ({
            columns = [],
            dataSource = [],
        }: {
            columns?: Array<{
                key?: string;
                title?: ReactNode;
                dataIndex?: string | string[];
                render?: (value: unknown, record: Record<string, unknown>, index: number) => ReactNode;
            }>;
            dataSource?: Array<Record<string, unknown>>;
        }) => (
            <table data-testid="antd-table">
                <thead>
                    <tr>
                        {columns.map((column, index) => (
                            <th key={String(column.key ?? column.dataIndex ?? index)}>{column.title}</th>
                        ))}
                    </tr>
                </thead>
                <tbody>
                    {dataSource.map((record, rowIndex) => (
                        <tr key={String(record.id ?? rowIndex)}>
                            {columns.map((column, columnIndex) => {
                                const value = Array.isArray(column.dataIndex)
                                    ? column.dataIndex.reduce<unknown>(
                                        (current, key) =>
                                            current && typeof current === 'object'
                                                ? (current as Record<string, unknown>)[key]
                                                : undefined,
                                        record,
                                    )
                                    : typeof column.dataIndex === 'string'
                                        ? record[column.dataIndex]
                                        : undefined;
                                const content = column.render
                                    ? column.render(value, record, rowIndex)
                                    : (value as ReactNode);
                                return (
                                    <td key={String(column.key ?? column.dataIndex ?? columnIndex)}>
                                        {content}
                                    </td>
                                );
                            })}
                        </tr>
                    ))}
                </tbody>
            </table>
        ),
    };
});

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: (...args: unknown[]) => useApiGetMock(...args),
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
    PageSurface: ({
        children,
    }: {
        children: ReactNode;
    }) => <section data-testid="page-surface">{children}</section>,
}));

vi.mock('@/components/ui/LocalDateTimeText', () => ({
    LocalDateTimeText: ({ value }: { value: string }) => <time dateTime={value}>{value}</time>,
}));

vi.mock('@/components/illustrations/DashboardIllustrations', () => ({
    RequestsOverviewGlyph: (props: Record<string, unknown>) => <span {...props}>requests-glyph</span>,
    ServiceWorkspaceGlyph: (props: Record<string, unknown>) => <span {...props}>service-glyph</span>,
    VirtualMachinesOverviewGlyph: (props: Record<string, unknown>) => <span {...props}>vms-glyph</span>,
}));

vi.mock('react-markdown', () => ({
    default: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock('remark-gfm', () => ({
    default: [],
}));

vi.mock('rehype-sanitize', () => ({
    default: [],
}));

vi.mock('@/components/workbench/WorkbenchDetailModal', () => ({
    WorkbenchDetailModal: ({
        open,
        title,
        children,
        footer,
    }: {
        open?: boolean;
        title?: React.ReactNode;
        children?: React.ReactNode;
        footer?: React.ReactNode;
    }) =>
        open ? (
            <div className="ant-modal">
                <div>{title}</div>
                <div>{children}</div>
                <div>{footer}</div>
            </div>
        ) : null,
}));

vi.mock('@/components/auth/PermissionGuard', () => ({
    PermissionGuard: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock('../hooks/useServicesManagementController', () => ({
    ALL_SYSTEMS_FILTER: '__all__',
    useServicesManagementController: () => {
        const [form] = Form.useForm();
        const [editForm] = Form.useForm();

        return {
            messageContextHolder: null,
            createOpen: false,
            editOpen: false,
            editingService: null,
            filters: {
                search: '',
                systemId: '__all__',
            },
            hasActiveFilters: false,
            activeSystemId: '__all__',
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
                    {
                        id: 'svc-2',
                        system_id: 'sys-1',
                        system_name: 'System A',
                        name: 'Payments',
                        description: 'settlement',
                        next_instance_index: 8,
                        created_at: '2026-03-17T00:00:00Z',
                    },
                ],
                pagination: { page: 1, per_page: 20, total: 2 },
            },
            isLoading: false,
            refetch: vi.fn(),
            applyFilters: applyFiltersMock,
            clearFilters: clearFiltersMock,
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
        applyFiltersMock.mockReset();
        clearFiltersMock.mockReset();
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

    it('renders compact row actions with accessible names', () => {
        render(<ServicesManagementContent />);

        expect(screen.getByRole('button', { name: 'Details Service A' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Edit Service A' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Request VM Service A' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Delete Service A' })).toBeInTheDocument();
    });

    it('submits quick search only when the user confirms it', () => {
        render(<ServicesManagementContent />);

        fireEvent.change(screen.getByTestId('services-quick-search'), {
            target: { value: 'payment' },
        });

        expect(applyFiltersMock).not.toHaveBeenCalled();
        fireEvent.keyDown(screen.getByTestId('services-quick-search'), {
            key: 'Enter',
            code: 'Enter',
        });

        expect(applyFiltersMock).toHaveBeenCalledWith({
            search: 'payment',
            systemId: '__all__',
        });
    });

    it('shows advanced filters and submits them explicitly', () => {
        render(<ServicesManagementContent />);

        fireEvent.click(screen.getByRole('button', { name: 'Advanced search' }));
        fireEvent.click(screen.getByTestId('services-advanced-search-submit'));

        expect(applyFiltersMock).toHaveBeenCalledWith({
            search: '',
            systemId: '__all__',
        });
    });

    it('renders the owning system as a clickable link without exposing the raw system id', () => {
        render(<ServicesManagementContent />);

        fireEvent.click(screen.getByTestId('service-action-open-system-svc-1'));

        expect(pushMock).toHaveBeenCalledWith('/systems?detail_system_id=sys-1');
        expect(screen.queryByText('sys-1')).not.toBeInTheDocument();
    });

    it('shows a service context modal with related VMs and requests, then opens the VM workspace', async () => {
        render(<ServicesManagementContent />);

        fireEvent.click(screen.getByTestId('service-action-detail-svc-1'));

        const detailModalTitle = await screen.findByText('Service Context');
        const detailModal = detailModalTitle.closest('.ant-modal') as HTMLElement | null;

        expect(detailModal).not.toBeNull();

        const modalQueries = within(detailModal!);
        expect(modalQueries.getByText('Service Context')).toBeInTheDocument();
        expect(modalQueries.getAllByText('Service Context')).toHaveLength(1);
        fireEvent.click(modalQueries.getByRole('button', { name: 'System A' }));
        expect(pushMock).toHaveBeenCalledWith('/systems?detail_system_id=sys-1');
        expect(modalQueries.getByText('Visible Virtual Machines')).toBeInTheDocument();
        expect(modalQueries.getByText('My Recent Requests')).toBeInTheDocument();
        expect(modalQueries.getAllByText('vm-a').length).toBeGreaterThan(0);
        expect(modalQueries.getAllByText('Create').length).toBeGreaterThan(0);
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
        const openWorkspaceButton = modalQueries.getByRole('button', {
            name: 'Open VM Workspace',
        });

        fireEvent.click(openWorkspaceButton);

        await waitFor(() => {
            expect(pushMock).toHaveBeenCalledWith('/vms?system_id=sys-1&service_id=svc-1');
        });
    });

    it('opens the service detail modal from deep link context', async () => {
        window.history.replaceState({}, '', '/services?system_id=sys-1&detail_service_id=svc-1');

        render(<ServicesManagementContent />);

        const detailModalTitle = await screen.findByText('Service Context');
        const detailModal = detailModalTitle.closest('.ant-modal') as HTMLElement | null;
        expect(detailModal).not.toBeNull();

        const modalQueries = within(detailModal!);
        expect(modalQueries.getByText('Service A')).toBeInTheDocument();
        expect(modalQueries.getAllByText('System A').length).toBeGreaterThan(0);
        expect(modalQueries.getAllByText('vm-a').length).toBeGreaterThan(0);
    });
});
