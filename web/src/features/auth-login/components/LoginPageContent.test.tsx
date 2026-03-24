/* eslint-disable @next/next/no-img-element */

import { render, screen } from '@testing-library/react';
import type { ImgHTMLAttributes } from 'react';
import { describe, expect, it, vi } from 'vitest';

const loginMock = vi.fn();
const startExternalLoginMock = vi.fn();
const submitExternalCredentialLoginMock = vi.fn();

vi.mock('next/image', () => ({
    default: (props: ImgHTMLAttributes<HTMLImageElement>) => <img {...props} alt={props.alt ?? ''} />,
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string) => {
            const labels: Record<string, string> = {
                'app.name': 'Shepherd',
                'app.subtitle': 'Infrastructure workflow hub',
                'auth.username': 'Username',
                'auth.password': 'Password',
                'auth.login': 'Sign in',
                'auth.or_continue_with': 'Or continue with',
                'validation.username_required': 'Username is required',
                'validation.username_min': 'Username is too short',
                'validation.password_required': 'Password is required',
                'message.loading': 'Loading',
            };
            return labels[key] ?? key;
        },
    }),
}));

vi.mock('@/hooks/useAuth', () => ({
    useAuth: () => ({
        login: loginMock,
        startExternalLogin: startExternalLoginMock,
        submitExternalCredentialLogin: submitExternalCredentialLoginMock,
    }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: () => ({
        data: { items: [] },
        isLoading: false,
    }),
}));

import LoginPageContent from './LoginPageContent';

describe('LoginPageContent', () => {
    it('renders the login shell and primary credential fields', async () => {
        render(<LoginPageContent />);

        expect(await screen.findByTestId('login-page')).toBeVisible();
        expect(screen.getByText('Shepherd')).toBeVisible();
        expect(screen.getByPlaceholderText('Username')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Password')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Sign in' })).toBeVisible();
    });
});
