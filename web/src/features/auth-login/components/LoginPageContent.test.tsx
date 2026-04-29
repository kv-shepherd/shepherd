/* eslint-disable @next/next/no-img-element */

import { render, screen, waitFor } from '@testing-library/react';
import type { ImgHTMLAttributes } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const loginMock = vi.fn();
const startExternalLoginMock = vi.fn();
const submitExternalCredentialLoginMock = vi.fn();
const changeLanguageMock = vi.fn();
const replaceMock = vi.fn();

const authHookState = {
    isAuthenticated: false,
    forcePasswordChange: false,
};

const authStoreState = {
    hasHydrated: true,
    hasValidatedSession: true,
};

vi.mock('next/image', () => ({
    default: (props: ImgHTMLAttributes<HTMLImageElement>) => <img {...props} alt={props.alt ?? ''} />,
}));

vi.mock('next/navigation', () => ({
    useRouter: () => ({
        replace: replaceMock,
    }),
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
                'auth.dev_login_hint_description': 'Local development and Codespaces demos seed admin / admin on first bootstrap. If the password was already changed, use the updated credential.',
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
        isAuthenticated: authHookState.isAuthenticated,
        forcePasswordChange: authHookState.forcePasswordChange,
    }),
}));

vi.mock('@/hooks/useApiQuery', () => ({
    useApiGet: () => ({
        data: { items: [] },
        isLoading: false,
    }),
}));

vi.mock('@/stores/auth', () => ({
    useAuthStore: (selector: (state: typeof authStoreState) => unknown) => selector(authStoreState),
}));

import LoginPageContent from './LoginPageContent';

describe('LoginPageContent', () => {
    beforeEach(() => {
        authHookState.isAuthenticated = false;
        authHookState.forcePasswordChange = false;
        authStoreState.hasHydrated = true;
        authStoreState.hasValidatedSession = true;
        replaceMock.mockReset();
    });

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
        expect(screen.getByText('Local development and Codespaces demos seed admin / admin on first bootstrap. If the password was already changed, use the updated credential.')).toBeVisible();
    });

    it('redirects authenticated users away from /login', async () => {
        authHookState.isAuthenticated = true;

        render(<LoginPageContent />);

        await waitFor(() => {
            expect(replaceMock).toHaveBeenCalledWith('/');
        });
        expect(screen.queryByTestId('login-page')).not.toBeInTheDocument();
    });

    it('redirects force-password-change users to the password reset page', async () => {
        authHookState.isAuthenticated = true;
        authHookState.forcePasswordChange = true;

        render(<LoginPageContent />);

        await waitFor(() => {
            expect(replaceMock).toHaveBeenCalledWith('/auth/change-password');
        });
    });
});
