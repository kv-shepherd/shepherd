/* eslint-disable @next/next/no-img-element */

import { render, screen } from '@testing-library/react';
import type { ImgHTMLAttributes } from 'react';
import { describe, expect, it, vi } from 'vitest';

const loginMock = vi.fn();
const startExternalLoginMock = vi.fn();
const submitExternalCredentialLoginMock = vi.fn();
const changeLanguageMock = vi.fn();

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
                'auth.welcome_back': 'Welcome back',
                'auth.or_continue_with': 'Or continue with',
                'auth.dev_login_hint_title': 'Local demo sign-in',
                'auth.dev_login_hint_description': 'Use admin / admin for local development and Codespaces demos. First sign-in will require a password change.',
                'validation.username_required': 'Username is required',
                'validation.username_min': 'Username is too short',
                'validation.password_required': 'Password is required',
                'message.loading': 'Loading',
                'language.label': 'Language',
                'language.english': 'English',
                'language.chinese': 'Chinese',
                'language.short_english': 'EN',
                'language.short_chinese': 'ZH',
            };
            return labels[key] ?? key;
        },
        i18n: {
            language: 'en',
            resolvedLanguage: 'en',
            changeLanguage: changeLanguageMock,
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
        expect(screen.getByTestId('login-language-switcher')).toHaveTextContent('EN');
    });

    it('shows the local demo hint on localhost', async () => {
        render(<LoginPageContent />);

        expect(await screen.findByText('Local demo sign-in')).toBeVisible();
        expect(screen.getByText('Use admin / admin for local development and Codespaces demos. First sign-in will require a password change.')).toBeVisible();
    });
});
