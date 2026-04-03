import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, options?: { defaultValue?: string }) => {
            const labels: Record<string, string> = {
                'rbac.permissions.title': 'Permission Catalog',
                'rbac.permissions.subtitle': 'Available permission keys that roles can include',
                'rbac.permissions.table.permission': 'Permission',
                'rbac.permissions.table.use_case': 'Typical use',
                'rbac.permissions.catalog.vm_create.label': 'Request virtual machines',
                'rbac.permissions.catalog.vm_create.description': 'Submit approved requests to provision new VMs.',
                'rbac.scope.vm': 'VM',
                'rbac.scope.rbac': 'RBAC',
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: () => ({
        data: {
            items: [
                { key: 'vm:create', description: 'Submit approved requests to provision new VMs.' },
                { key: 'rbac:manage', description: 'Manage access control.' },
            ],
        },
        isLoading: false,
    }),
}));

import AdminPermissionsPage from './page';

describe('AdminPermissionsPage', () => {
    it('renders the page shell and catalog entries returned by the backend', () => {
        render(<AdminPermissionsPage />);

        expect(screen.getByTestId('admin-permissions-page')).toBeVisible();
        expect(screen.getByText('Permission Catalog')).toBeVisible();
        expect(screen.getByText('Permission')).toBeVisible();
        expect(screen.getByText('Request virtual machines')).toBeVisible();
        expect(screen.getByText('vm:create')).toBeVisible();
        expect(screen.getAllByText('VM').length).toBeGreaterThan(0);
    });

    it('filters the catalog through quick search only after submit', async () => {
        const user = userEvent.setup();
        render(<AdminPermissionsPage />);

        await user.type(screen.getByTestId('permissions-quick-search'), 'rbac');
        expect(screen.getByText('vm:create')).toBeVisible();

        await user.keyboard('{Enter}');
        expect(screen.queryByText('vm:create')).not.toBeInTheDocument();
        expect(screen.getByText('rbac:manage')).toBeVisible();
    });
});
