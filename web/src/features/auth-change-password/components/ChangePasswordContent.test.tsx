/* eslint-disable @next/next/no-img-element */

import { render, screen } from '@testing-library/react';
import type { ImgHTMLAttributes } from 'react';
import { describe, expect, it, vi } from 'vitest';

const submitMock = vi.fn();
const setErrorMock = vi.fn();

vi.mock('next/image', () => ({
    default: (props: ImgHTMLAttributes<HTMLImageElement>) => <img {...props} alt={props.alt ?? ''} />,
}));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string) => {
            const labels: Record<string, string> = {
                'auth.security': 'Security',
                'auth.change_password': 'Change Password',
                'auth.change_password_hint': 'Update your credentials before continuing.',
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

vi.mock('../hooks/useChangePasswordController', () => ({
    useChangePasswordController: () => ({
        loading: false,
        error: 'Current password is invalid',
        setError: setErrorMock,
        submit: submitMock,
    }),
}));

import { ChangePasswordContent } from './ChangePasswordContent';

describe('ChangePasswordContent', () => {
    it('renders the auth shell, error state, and submit action', () => {
        render(<ChangePasswordContent />);

        expect(screen.getByTestId('change-password-page')).toBeVisible();
        expect(screen.getByText('Security')).toBeVisible();
        expect(screen.getAllByText('Change Password').length).toBeGreaterThan(0);
        expect(screen.getByText('Update your credentials before continuing.')).toBeVisible();
        expect(screen.getByText('Current password is invalid')).toBeVisible();
        expect(screen.getByPlaceholderText('Current Password')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('New Password')).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Confirm Password')).toBeInTheDocument();
        expect(screen.getByTestId('change-password-submit')).toBeVisible();
    });
});
