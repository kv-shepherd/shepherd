/**
 * Authentication store (Zustand 5 — ADR-0020).
 *
 * Manages JWT token, user info, and auth state.
 * Persisted to localStorage under key 'shepherd-auth'.
 *
 * AGENTS.md §3.5: Exported for use in AuthGuard and API middleware.
 */
import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';
import type { components } from '@/types/api.gen';

/** UserInfo from OpenAPI generated types */
type UserInfo = components['schemas']['UserInfo'];

interface AuthState {
    // State
    token: string | null;
    user: UserInfo | null;
    isAuthenticated: boolean;
    forcePasswordChange: boolean;

    // Actions
    login: (token: string, user: UserInfo, forcePasswordChange?: boolean) => void;
    logout: () => void;
    updateUser: (user: UserInfo) => void;
    clearForcePasswordChange: () => void;
}

/** Zustand store key used in localStorage */
export const AUTH_STORAGE_KEY = 'shepherd-auth';

export const useAuthStore = create<AuthState>()(
    persist(
        (set) => ({
            // Initial state
            token: null,
            user: null,
            isAuthenticated: false,
            forcePasswordChange: false,

            // Actions
            login: (token, user, forcePasswordChange = false) =>
                set({
                    token,
                    user,
                    isAuthenticated: true,
                    forcePasswordChange,
                }),

            logout: () =>
                set({
                    token: null,
                    user: null,
                    isAuthenticated: false,
                    forcePasswordChange: false,
                }),

            updateUser: (user) => set({ user }),

            clearForcePasswordChange: () => set({ forcePasswordChange: false }),
        }),
        {
            name: AUTH_STORAGE_KEY,
            storage: createJSONStorage(() =>
                typeof window !== 'undefined' ? localStorage : {
                    getItem: () => null,
                    setItem: () => { },
                    removeItem: () => { },
                }
            ),
            partialize: (state) => ({
                token: state.token,
                user: state.user,
                isAuthenticated: state.isAuthenticated,
            }),
        }
    )
);
