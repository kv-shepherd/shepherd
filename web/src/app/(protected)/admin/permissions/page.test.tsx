import { render, screen } from '@testing-library/react';
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
            };
            return labels[key] ?? options?.defaultValue ?? key;
        },
    }),
}));

import AdminPermissionsPage from './page';

describe('AdminPermissionsPage', () => {
    it('renders the page shell and friendly permission catalog', () => {
        render(<AdminPermissionsPage />);

        expect(screen.getByTestId('admin-permissions-page')).toBeVisible();
        expect(screen.getByText('Permission Catalog')).toBeVisible();
        expect(screen.getByText('Permission')).toBeVisible();
        expect(screen.getByText('Request virtual machines')).toBeVisible();
        expect(screen.getByText('vm:create')).toBeVisible();
        expect(screen.getAllByText('VM').length).toBeGreaterThan(0);
    });
});
