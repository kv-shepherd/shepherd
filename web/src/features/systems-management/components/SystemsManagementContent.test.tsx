import { Form } from 'antd';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const openCreateModalMock = vi.fn();
const pushMock = vi.fn();
const useApiGetMock = vi.fn();
const useSystemsManagementControllerMock = vi.fn();

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
                            created_by: 'alice',
                            created_at: '2026-03-24T00:00:00Z',
                        },
                    ],
                    pagination: { total: 1 },
                },
                isLoading: false,
                refetch: vi.fn(),
                openCreateModal: openCreateModalMock,
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
                page: 1,
                pageSize: 20,
                setPage: vi.fn(),
                setPageSize: vi.fn(),
                data: { items: [], pagination: { total: 0 } },
                isLoading: false,
                refetch: vi.fn(),
                openCreateModal: openCreateModalMock,
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
        render(<SystemsManagementContent />);

        await waitFor(() => {
            expect(screen.getByTestId('system-service-link-svc-1')).toBeVisible();
        });

        fireEvent.click(screen.getByTestId('system-service-link-svc-1'));
        expect(pushMock).toHaveBeenCalledWith('/services?system_id=sys-1&detail_service_id=svc-1');
    }, 10000);

    it('opens the services workspace from the system detail modal', async () => {
        window.history.replaceState({}, '', '/systems?detail_system_id=sys-1');

        render(<SystemsManagementContent />);

        await waitFor(() => {
            expect(screen.getAllByText('Retail platform').length).toBeGreaterThan(0);
        });

        pushMock.mockClear();
        fireEvent.click(screen.getByRole('button', { name: 'Open Services' }));
        expect(pushMock).toHaveBeenCalledWith('/services?system_id=sys-1');
    }, 25000);
});
