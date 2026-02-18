import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

let currentUser: { permissions?: string[] } | null = null;

vi.mock('@/stores/auth', () => ({
    useAuthStore: (selector: (state: { user: typeof currentUser }) => unknown) =>
        selector({ user: currentUser }),
}));

import { PermissionGuard } from './PermissionGuard';

describe('PermissionGuard', () => {
    it('renders children when required permission exists', () => {
        currentUser = { permissions: ['vm:create'] };

        render(
            <PermissionGuard permission="vm:create">
                <div>allowed</div>
            </PermissionGuard>
        );

        expect(screen.getByText('allowed')).toBeInTheDocument();
    });

    it('renders fallback when permission is missing', () => {
        currentUser = { permissions: ['vm:read'] };

        render(
            <PermissionGuard permission="vm:create" fallback={<div>denied</div>}>
                <div>allowed</div>
            </PermissionGuard>
        );

        expect(screen.queryByText('allowed')).not.toBeInTheDocument();
        expect(screen.getByText('denied')).toBeInTheDocument();
    });
});

