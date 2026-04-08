import { Form } from 'antd';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const openCreateModalMock = vi.fn();
const pushMock = vi.fn();
const useApiGetMock = vi.fn();
const useSystemsManagementControllerMock = vi.fn();
const applyFiltersMock = vi.fn();
const clearFiltersMock = vi.fn();

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (
            key: string,
            options?: string | { defaultValue?: string; [key: string]: unknown },
        ) => {
            const labels: Record<string, string> = {
                'nav.systems': 'Systems',
                'systems.subtitle': 'Manage systems',
                'button.refresh': 'Refresh',
                'button.create': 'Create',
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
        Select: ({
            value,
            options = [],
            onChange,
            placeholder,
            'data-testid': dataTestId,
        }: {
            value?: string;
            options?: Array<{ value: string; label?: ReactNode }>;
            onChange?: (value?: string) => void;
            placeholder?: string;
            'data-testid'?: string;
        }) => (
            <select
                data-testid={dataTestId}
                aria-label={placeholder}
                value={value ?? ''}
                onChange={(event) => onChange?.(event.target.value || undefined)}
            >
                <option value="">{placeholder}</option>
                {options.map((option) => (
                    <option key={option.value} value={option.value}>
                        {typeof option.label === 'string' ? option.label : option.value}
                    </option>
                ))}
            </select>
        ),
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
    PageSurface: ({ children }: { children: ReactNode }) => (
        <section data-testid="page-surface">{children}</section>
    ),
}));

vi.mock('@/components/ui/LocalDateTimeText', () => ({
    LocalDateTimeText: ({ value }: { value: string }) => <time dateTime={value}>{value}</time>,
}));

vi.mock('@/components/workbench/WorkbenchDetailModal', () => ({
    WorkbenchDetailModal: ({
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
            <div className="ant-modal">
                <div>{title}</div>
                <div>{children}</div>
                <div>{footer}</div>
            </div>
        ) : null,
}));

vi.mock('@/components/illustrations/DashboardIllustrations', () => ({
    SystemsOverviewGlyph: (props: Record<string, unknown>) => <span {...props}>systems-glyph</span>,
}));

vi.mock('@/features/setup-guide/components/SetupGuideCard', () => ({
    SetupGuideCard: ({ variant }: { variant: string }) => <div>{`setup-guide-${variant}`}</div>,
}));

vi.mock('@/features/setup-guide/hooks/useSetupGuide', () => ({
    useSetupGuide: () => ({
        systemsTotal: 0,
        servicesTotal: 0,
        vmsTotal: 0,
        namespacesTotal: 0,
        templatesTotal: 0,
        instanceSizesTotal: 0,
        canCreateSystem: true,
        canCreateService: true,
        canCreateVM: true,
        canManageNamespaces: true,
        canManageTemplates: true,
        canManageInstanceSizes: true,
        systemReady: false,
        serviceReady: false,
        prerequisitesReady: false,
        vmRequestReady: false,
        hasRequestedFirstVM: false,
        isLoading: false,
    }),
}));

vi.mock('./SystemMembersModal', () => ({
    SystemMembersModal: () => null,
}));

vi.mock('../hooks/useSystemsManagementController', () => ({
    useSystemsManagementController: (...args: unknown[]) =>
        useSystemsManagementControllerMock(...args),
}));

import { SystemsManagementContent } from './SystemsManagementContent';

describe('SystemsManagementContent', () => {
    beforeEach(() => {
        openCreateModalMock.mockReset();
        pushMock.mockReset();
        applyFiltersMock.mockReset();
        clearFiltersMock.mockReset();
        useSystemsManagementControllerMock.mockImplementation(() => {
            const [form] = Form.useForm();
            const [editForm] = Form.useForm();
            return {
                messageContextHolder: null,
                createOpen: false,
                editOpen: false,
                editingSystem: null,
                deleteOpen: false,
                deletingSystem: null,
                deleteConfirmName: '',
                setDeleteConfirmName: vi.fn(),
                form,
                editForm,
                filters: {
                    search: '',
                    createdBy: '',
                    serviceId: '',
                    memberId: '',
                },
                hasActiveFilters: false,
                page: 1,
                pageSize: 20,
                setPage: vi.fn(),
                setPageSize: vi.fn(),
                data: {
                    items: [
                        {
                            id: 'sys-1',
                            name: 'shop',
                            description: 'Retail platform',
                            created_by: 'user-alice',
                            created_by_display_name: 'Alice Ops',
                            created_by_username: 'alice.ops',
                            created_at: '2026-03-24T00:00:00Z',
                        },
                        {
                            id: 'sys-2',
                            name: 'finance',
                            description: 'Finance workspace',
                            created_by: 'user-bob',
                            created_by_display_name: 'Bob Finance',
                            created_by_username: 'bob.finance',
                            created_at: '2026-03-25T00:00:00Z',
                        },
                    ],
                    pagination: { total: 2 },
                },
                systemFilterOptions: {
                    creators: [{ value: 'user-alice', label: 'Alice Ops · alice.ops' }],
                    services: [{ value: 'svc-1', label: 'shop / billing' }],
                    members: [{ value: 'user-bob', label: 'Bob Builder · bob.builder@example.com' }],
                },
                systemFilterOptionsLoading: false,
                isLoading: false,
                refetch: vi.fn(),
                openCreateModal: openCreateModalMock,
                applyFilters: applyFiltersMock,
                clearFilters: clearFiltersMock,
                closeCreateModal: vi.fn(),
                openDeleteModal: vi.fn(),
                openEditModal: vi.fn(),
                closeEditModal: vi.fn(),
                closeDeleteModal: vi.fn(),
                submitCreate: vi.fn(),
                submitEdit: vi.fn(),
                submitDelete: vi.fn(),
                createPending: false,
                updatePending: false,
                deletePending: false,
                membersOpen: false,
                membersSystem: null,
                openMembersModal: vi.fn(),
                closeMembersModal: vi.fn(),
            };
        });
        useApiGetMock.mockReturnValue({
            data: {
                items: [
                    {
                        id: 'svc-1',
                        system_id: 'sys-1',
                        system_name: 'shop',
                        name: 'billing',
                        description: 'Billing service',
                        next_instance_index: 4,
                        created_at: '2026-03-24T00:00:00Z',
                    },
                ],
                pagination: { total: 1 },
            },
            isLoading: false,
        });
        window.history.replaceState({}, '', '/systems');
    });

    it('shows the setup guide when no systems exist', () => {
        useSystemsManagementControllerMock.mockImplementationOnce(() => {
            const [form] = Form.useForm();
            const [editForm] = Form.useForm();
            return {
                messageContextHolder: null,
                createOpen: false,
                editOpen: false,
                editingSystem: null,
                deleteOpen: false,
                deletingSystem: null,
                deleteConfirmName: '',
                setDeleteConfirmName: vi.fn(),
                form,
                editForm,
                filters: {
                    search: '',
                    createdBy: '',
                    serviceId: '',
                    memberId: '',
                },
                hasActiveFilters: false,
                page: 1,
                pageSize: 20,
                setPage: vi.fn(),
                setPageSize: vi.fn(),
                data: { items: [], pagination: { total: 0 } },
                systemFilterOptions: {
                    creators: [],
                    services: [],
                    members: [],
                },
                systemFilterOptionsLoading: false,
                isLoading: false,
                refetch: vi.fn(),
                openCreateModal: openCreateModalMock,
                applyFilters: applyFiltersMock,
                clearFilters: clearFiltersMock,
                closeCreateModal: vi.fn(),
                openDeleteModal: vi.fn(),
                openEditModal: vi.fn(),
                closeEditModal: vi.fn(),
                closeDeleteModal: vi.fn(),
                submitCreate: vi.fn(),
                submitEdit: vi.fn(),
                submitDelete: vi.fn(),
                createPending: false,
                updatePending: false,
                deletePending: false,
                membersOpen: false,
                membersSystem: null,
                openMembersModal: vi.fn(),
                closeMembersModal: vi.fn(),
            };
        });
        render(<SystemsManagementContent />);

        expect(screen.getByText('setup-guide-systems')).toBeVisible();
    });

    it('auto-opens the create modal from setup intent links', async () => {
        window.history.replaceState({}, '', '/systems?intent=create-system');

        render(<SystemsManagementContent />);

        await waitFor(() => {
            expect(openCreateModalMock).toHaveBeenCalledTimes(1);
        });
    });

    it('shows related service names in the systems list and lets the user open a service detail', async () => {
        const user = userEvent.setup();
        render(<SystemsManagementContent />);

        await waitFor(() => {
            expect(screen.getAllByTestId('system-service-link-svc-1').length).toBeGreaterThan(0);
        });

        await user.click(screen.getAllByTestId('system-service-link-svc-1')[0]);
        expect(pushMock).toHaveBeenCalledWith('/services?system_id=sys-1&detail_service_id=svc-1');
    });

    it('submits quick search only when the user confirms it', async () => {
        const user = userEvent.setup();
        render(<SystemsManagementContent />);

        await user.type(screen.getByTestId('systems-quick-search'), 'finance');

        expect(applyFiltersMock).not.toHaveBeenCalled();
        await user.keyboard('{Enter}');

        expect(applyFiltersMock).toHaveBeenCalledWith({
            search: 'finance',
            createdBy: '',
            serviceId: '',
            memberId: '',
        });
    });

    it('shows advanced search and submits creator, service, and member filters', async () => {
        const user = userEvent.setup();
        render(<SystemsManagementContent />);

        await user.click(screen.getByTestId('systems-search-filters-toggle'));

        await user.selectOptions(screen.getByTestId('systems-filter-created-by'), 'user-alice');
        await user.selectOptions(screen.getByTestId('systems-filter-service'), 'svc-1');
        await user.selectOptions(screen.getByTestId('systems-filter-member'), 'user-bob');
        await user.click(screen.getByTestId('systems-advanced-search-submit'));

        expect(applyFiltersMock).toHaveBeenCalledWith({
            search: '',
            createdBy: 'user-alice',
            serviceId: 'svc-1',
            memberId: 'user-bob',
        });
    });

    it('opens the services workspace from the system detail modal', async () => {
        const user = userEvent.setup();
        window.history.replaceState({}, '', '/systems?detail_system_id=sys-1');

        render(<SystemsManagementContent />);

        await waitFor(() => {
            expect(screen.getAllByText('Retail platform').length).toBeGreaterThan(0);
        });

        pushMock.mockClear();
        await user.click(screen.getByRole('button', { name: 'Open Services' }));
        expect(pushMock).toHaveBeenCalledWith('/services?system_id=sys-1');
    });
});
