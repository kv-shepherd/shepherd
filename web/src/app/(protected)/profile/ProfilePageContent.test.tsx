import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const mutationState = vi.hoisted(() => ({
    mutate: vi.fn(),
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string) => {
            const labels: Record<string, string> = {
                'nav.profile': 'My Profile',
                'profile.subtitle': 'View your account details and manage settings.',
                'profile.account_info': 'Account Information',
                'table.username': 'Username',
                'table.role': 'Role',
                'table.email': 'Email',
                'table.created_at': 'Joined',
                'auth.security': 'Security',
                'auth.security_subtitle': 'Manage your login credentials.',
                'auth.change_password': 'Change Password',
                'auth.current_password': 'Current Password',
                'auth.new_password': 'New Password',
                'auth.confirm_password': 'Confirm Password',
                'validation.password_required': 'Password is required',
                'validation.password_min': 'Password is too short',
                'validation.confirm_password_required': 'Please confirm password',
                'validation.password_mismatch': 'Passwords do not match',
            };
            return labels[key] ?? key;
        },
    }),
}));

vi.mock('@/lib/api/useApiGet', () => ({
    useApiGet: () => ({
        data: {
            username: 'alice',
            email: 'alice@example.com',
            role: 'platform-admin',
            created_at: '2026-03-16T00:00:00Z',
        },
    }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiMutation: () => ({
        mutate: mutationState.mutate,
        isPending: false,
    }),
}));

vi.mock('@/lib/hooks/useMessage', () => ({
    useMessage: () => ({
        messageApi: {
            success: vi.fn(),
        },
        messageContextHolder: null,
    }),
}));

vi.mock('@/lib/api/client', () => ({
    api: {
        GET: vi.fn(),
        POST: vi.fn(),
    },
}));

import ProfilePage from './ProfilePageContent';

describe('ProfilePageContent', () => {
    beforeEach(() => {
        mutationState.mutate.mockReset();
    });

    it('renders profile sections and opens the change password modal', () => {
        render(<ProfilePage />);

        expect(screen.getByTestId('profile-page')).toBeVisible();
        expect(screen.getByText('My Profile')).toBeVisible();
        expect(screen.getByText('Account Information')).toBeVisible();
        expect(screen.getByText('alice@example.com')).toBeVisible();
        expect(screen.getByTestId('change-password-button')).toBeVisible();

        fireEvent.click(screen.getByTestId('change-password-button'));

        expect(screen.getByTestId('change-password-modal')).toBeVisible();
        expect(screen.getByTestId('change-password-current-input')).toBeInTheDocument();
        expect(screen.getByTestId('change-password-new-input')).toBeInTheDocument();
        expect(screen.getByTestId('change-password-confirm-input')).toBeInTheDocument();
    });
});
