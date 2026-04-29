/**
 * Authentication store (Zustand 5 — ADR-0020).
 *
 * Manages user info and auth state.
 * Persisted schema is intentionally empty so browser storage cannot retain tokens.
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
    user: UserInfo | null;
    isAuthenticated: boolean;
    forcePasswordChange: boolean;
    hasHydrated: boolean;
    hasValidatedSession: boolean;

    // Actions
    login: (user: UserInfo, forcePasswordChange?: boolean) => void;
    restoreSession: (user: UserInfo) => void;
    logout: () => void;
    updateUser: (user: UserInfo) => void;
    clearForcePasswordChange: () => void;
    setHasHydrated: (value: boolean) => void;
    setHasValidatedSession: (value: boolean) => void;
}

/** Zustand store key used in localStorage */
export const AUTH_STORAGE_KEY = 'shepherd-auth';

export const useAuthStore = create<AuthState>()(
    persist(
        (set) => ({
            // Initial state
            user: null,
            isAuthenticated: false,
            forcePasswordChange: false,
            hasHydrated: false,
            hasValidatedSession: false,

            // Actions
            login: (user, forcePasswordChange = false) =>
                set({
                    user,
                    isAuthenticated: true,
                    forcePasswordChange,
                    hasValidatedSession: true,
                }),

            restoreSession: (user) =>
                set({
                    user,
                    isAuthenticated: true,
                    forcePasswordChange: user.force_password_change ?? false,
                    hasValidatedSession: true,
                }),

            logout: () =>
                set({
                    user: null,
                    isAuthenticated: false,
                    forcePasswordChange: false,
                    hasValidatedSession: true,
                }),

            updateUser: (user) =>
                set((state) => ({
                    user,
                    forcePasswordChange: user.force_password_change ?? state.forcePasswordChange,
                })),

            clearForcePasswordChange: () =>
                set((state) => ({
                    forcePasswordChange: false,
                    user: state.user
                        ? { ...state.user, force_password_change: false }
                        : state.user,
                })),

            setHasHydrated: (value) => set({ hasHydrated: value }),
            setHasValidatedSession: (value) => set({ hasValidatedSession: value }),
        }),
        {
            name: AUTH_STORAGE_KEY,
            version: 1,
            storage: createJSONStorage(() =>
                typeof window !== 'undefined' ? localStorage : {
                    getItem: () => null,
                    setItem: () => { },
                    removeItem: () => { },
                }
            ),
            migrate: () => ({}),
            partialize: () => ({}),
            onRehydrateStorage: () => (state) => {
                state?.setHasHydrated(true);
            },
        }
    )
);
