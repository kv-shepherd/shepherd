import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const authState = vi.hoisted(() => ({
    user: {
        permissions: ['platform:admin'],
    },
}));

vi.mock('@/stores/auth', () => ({
    useAuthStore: (selector: (state: { user: typeof authState.user }) => unknown) =>
        selector({ user: authState.user }),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (
            key: string,
            options?: string | { defaultValue?: string; [key: string]: unknown },
        ) => {
            const labels: Record<string, string> = {
                'adoptions.title': 'Pending Adoptions',
                'adoptions.subtitle': 'Review live resources',
                'adoptions.table.resource': 'Resource',
                'adoptions.table.location': 'Location',
                'adoptions.table.labels': 'Labels',
                'adoptions.table.discovered': 'Discovered',
                'adoptions.status.PENDING': 'Pending',
                'adoptions.status.ADOPTED': 'Adopted',
                'adoptions.action.adopt': 'Adopt',
                'common:button.reject': 'Reject',
                'common:button.refresh': 'Refresh',
                'common:button.search': 'Search',
                'common:table.status': 'Status',
                'common:table.actions': 'Actions',
                'common:table.total': 'Total {{total}} items',
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

vi.mock('../hooks/useAdminAdoptionsController', () => ({
    useAdminAdoptionsController: () => ({
        data: {
            items: [
                {
                    id: 'pa-1',
                    cluster_id: 'cluster-a',
                    namespace: 'team-a',
                    resource_name: 'vm-live-a',
                    resource_type: 'VirtualMachine',
                    status: 'PENDING',
                    discovered_by: 'system:vm-adoption-discovery',
                    labels: {
                        'shepherd.io/service-id': 'svc-1',
                    },
                    created_at: '2026-03-17T00:00:00Z',
                    updated_at: '2026-03-17T00:00:00Z',
                },
                {
                    id: 'pa-2',
                    cluster_id: 'cluster-b',
                    namespace: 'team-b',
                    resource_name: 'vm-adopted-b',
                    resource_type: 'VirtualMachine',
                    status: 'ADOPTED',
                    discovered_by: 'admin',
                    labels: {},
                    created_at: '2026-03-18T00:00:00Z',
                    updated_at: '2026-03-18T00:00:00Z',
                },
            ],
            pagination: { total: 2, page: 1, per_page: 20 },
        },
        isLoading: false,
        refetch: vi.fn(),
        page: 1,
        setPage: vi.fn(),
        search: '',
        statusFilter: 'PENDING',
        clusterFilter: '',
        namespaceFilter: '',
        changeSearch: vi.fn(),
        changeStatusFilter: vi.fn(),
        changeClusterFilter: vi.fn(),
        changeNamespaceFilter: vi.fn(),
        clearFilters: vi.fn(),
        decision: null,
        openDecision: vi.fn(),
        closeDecision: vi.fn(),
        setDecisionReason: vi.fn(),
        submitDecision: vi.fn(),
        decisionPending: false,
    }),
}));

import { AdminPendingAdoptionsContent } from './AdminPendingAdoptionsContent';

describe('AdminPendingAdoptionsContent', () => {
    it('renders the pending adoption table and platform admin actions', () => {
        authState.user = {
            permissions: ['platform:admin'],
        };

        render(<AdminPendingAdoptionsContent />);

        expect(screen.getByTestId('admin-pending-adoptions-page')).toBeVisible();
        expect(screen.getByText('Pending Adoptions')).toBeVisible();
        expect(screen.getByTestId('adoptions-refresh-btn')).toBeVisible();
        expect(screen.getByText('vm-live-a')).toBeVisible();
        expect(screen.getByText('cluster-a')).toBeVisible();
        expect(screen.getByText('service-id: svc-1')).toBeVisible();
        expect(screen.getByTestId('adoption-action-adopt-pa-1')).toBeVisible();
        expect(screen.getByTestId('adoption-action-reject-pa-1')).toBeVisible();
        expect(screen.queryByTestId('adoption-action-adopt-pa-2')).not.toBeInTheDocument();
    });

    it('hides decision actions from non-platform administrators', () => {
        authState.user = {
            permissions: ['cluster:read'],
        };

        render(<AdminPendingAdoptionsContent />);

        expect(screen.getByText('vm-live-a')).toBeVisible();
        expect(screen.queryByTestId('adoption-action-adopt-pa-1')).not.toBeInTheDocument();
        expect(screen.queryByTestId('adoption-action-reject-pa-1')).not.toBeInTheDocument();
    });
});
