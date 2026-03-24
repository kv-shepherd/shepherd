import { Form } from 'antd';
import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const openCreateModalMock = vi.fn();
const pushMock = vi.fn();

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
    useSystemsManagementController: () => {
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
    },
}));

import { SystemsManagementContent } from './SystemsManagementContent';

describe('SystemsManagementContent', () => {
    beforeEach(() => {
        openCreateModalMock.mockReset();
        pushMock.mockReset();
        window.history.replaceState({}, '', '/systems');
    });

    it('shows the setup guide when no systems exist', () => {
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
});
