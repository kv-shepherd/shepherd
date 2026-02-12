/**
 * Auth hooks for login, logout, and authentication state.
 *
 * AGENTS.md §8.1: Initialize app once, not per mount.
 * Uses the generated OpenAPI types for type-safe auth calls.
 */
'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { App } from 'antd';
import { useTranslation } from 'react-i18next';
import { useAuthStore } from '@/stores/auth';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';
import type { ApiErrorResponse } from './useApiQuery';

interface LoginPayload {
    username: string;
    password: string;
}

type UserInfo = components['schemas']['UserInfo'];

export function useAuth() {
    const router = useRouter();
    const { message } = App.useApp();
    const { t } = useTranslation('errors');

    const { token, user, isAuthenticated, forcePasswordChange, login, logout: clearAuth } =
        useAuthStore();

    const handleLogin = useCallback(
        async (payload: LoginPayload) => {
            // POST /auth/login (baseUrl already includes /api/v1)
            const { data, error } = await api.POST('/auth/login', {
                body: payload,
            });

            if (error) {
                const apiError = error as unknown as ApiErrorResponse;
                message.error(t(apiError.code ?? 'INVALID_CREDENTIALS'));
                throw apiError;
            }

            if (data) {
                // Token is now auto-attached via middleware from localStorage.
                // Fetch user info after login to get full user profile.
                // FIX: explicitly pass token in header because localStorage might not be updated yet for middleware
                const { data: userInfo } = await api.GET('/auth/me', {
                    headers: {
                        Authorization: `Bearer ${data.token}`,
                    },
                });

                if (userInfo) {
                    // Store token + user in Zustand (persisted to localStorage)
                    login(data.token, userInfo, data.force_password_change ?? false);

                    if (data.force_password_change) {
                        router.push('/auth/change-password');
                    } else {
                        router.push('/dashboard');
                    }
                } else {
                    // If we can't fetch user info, still store token and redirect
                    const fallbackUser: UserInfo = {
                        id: payload.username,
                        username: payload.username,
                    };
                    login(data.token, fallbackUser, data.force_password_change ?? false);
                    router.push('/dashboard');
                }
            }
        },
        [login, message, router, t]
    );

    const handleLogout = useCallback(() => {
        clearAuth();
        router.push('/login');
    }, [clearAuth, router]);

    return {
        token,
        user,
        isAuthenticated,
        forcePasswordChange,
        login: handleLogin,
        logout: handleLogout,
    };
}
