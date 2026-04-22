import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const pushMock = vi.fn();
const setupGuideState = vi.hoisted(() => ({
    canManageNamespaces: true,
}));

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        push: pushMock,
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
        canManageNamespaces: setupGuideState.canManageNamespaces,
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

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string) => {
            const labels: Record<string, string> = {
                'namespaces.title': 'Namespaces',
                'namespaces.subtitle': 'Manage namespace registry entries',
                'namespaces.table.namespace': 'Namespace',
                'namespaces.filter_env': 'Environment',
                'common:button.refresh': 'Refresh',
                'namespaces.add': 'Add Namespace',
                'common:table.name': 'Name',
                'namespaces.environment': 'Environment',
                'namespaces.enabled': 'Enabled',
                'namespaces.enabled_yes': 'Yes',
                'namespaces.enabled_no': 'No',
                'namespaces.created_by_hint': 'Registry author',
                'common:table.created_by': 'Created By',
                'common:table.created_at': 'Created',
                'common:table.actions': 'Actions',
                'common:button.detail': 'Detail',
                'common:button.edit': 'Edit',
                'common:button.delete': 'Delete',
                'common:table.total': 'Total',
            };
            return labels[key] ?? key;
        },
    }),
}));

vi.mock('../hooks/useAdminNamespacesController', () => ({
    useAdminNamespacesController: () => ({
        messageContextHolder: null,
        data: {
            items: [
                {
                    id: 'ns-1',
                    name: 'team-prod',
                    description: 'Production namespace',
                    environment: 'prod',
                    enabled: true,
                    created_by: 'alice',
                    created_at: '2026-03-17T00:00:00Z',
                },
            ],
            pagination: { total: 1, page: 1, per_page: 20 },
        },
        isLoading: false,
        page: 1,
        envFilter: '',
        enabledFilter: '',
        search: '',
        changeSearch: vi.fn(),
        changeEnvFilter: vi.fn(),
        changeEnabledFilter: vi.fn(),
        refetch: vi.fn(),
        setPage: vi.fn(),
        openCreateModal: vi.fn(),
        openEditModal: vi.fn(),
        openDeleteModal: vi.fn(),
        createOpen: false,
        editOpen: false,
        deleteOpen: false,
        createPending: false,
        updatePending: false,
        deletePending: false,
        createForm: undefined,
        editForm: undefined,
        submitCreate: vi.fn(),
        submitUpdate: vi.fn(),
        submitDelete: vi.fn(),
        closeCreateModal: vi.fn(),
        closeEditModal: vi.fn(),
        closeDeleteModal: vi.fn(),
        deleteConfirmName: '',
        deletingNs: null,
        editingNs: null,
        setDeleteConfirmName: vi.fn(),
    }),
}));

import { AdminNamespacesContent } from './AdminNamespacesContent';

describe('AdminNamespacesContent', () => {
    beforeEach(() => {
        pushMock.mockReset();
        setupGuideState.canManageNamespaces = true;
    });

    it('renders the page shell and primary namespace actions', () => {
        render(<AdminNamespacesContent />);

        expect(screen.getByTestId('admin-namespaces-page')).toBeVisible();
        expect(screen.getByText('Namespaces')).toBeVisible();
        expect(screen.getByTestId('namespaces-refresh-btn')).toBeVisible();
        expect(screen.getByTestId('namespace-create-button')).toBeVisible();
        expect(screen.getByText('team-prod')).toBeVisible();
        expect(screen.getByText('Production namespace')).toBeVisible();
    });

    it('hides mutating namespace controls for read-only admins', () => {
        setupGuideState.canManageNamespaces = false;

        render(<AdminNamespacesContent />);

        expect(screen.queryByTestId('namespace-create-button')).not.toBeInTheDocument();
        expect(screen.getByTestId('namespace-action-detail-ns-1')).toBeVisible();
        expect(screen.queryByTestId('namespace-action-edit-ns-1')).not.toBeInTheDocument();
        expect(screen.queryByTestId('namespace-action-delete-ns-1')).not.toBeInTheDocument();
    });
});
