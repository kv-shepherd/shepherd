'use client';

import { useEffect, useRef } from 'react';

import { api } from '@/lib/api/client';
import { useAuthStore } from '@/stores/auth';

export function SessionBootstrap() {
    const hasHydrated = useAuthStore((state) => state.hasHydrated);
    const hasValidatedSession = useAuthStore((state) => state.hasValidatedSession);
    const restoreSession = useAuthStore((state) => state.restoreSession);
    const clearAuth = useAuthStore((state) => state.logout);
    const setHasValidatedSession = useAuthStore((state) => state.setHasValidatedSession);
    const startedRef = useRef(false);

    useEffect(() => {
        if (!hasHydrated || hasValidatedSession || startedRef.current) {
            return;
        }

        startedRef.current = true;
        let cancelled = false;

        void (async () => {
            try {
                const { data, error } = await api.GET('/auth/me');
                if (cancelled) {
                    return;
                }
                if (error || !data) {
                    clearAuth();
                    return;
                }
                restoreSession(data);
            } catch {
                if (!cancelled) {
                    clearAuth();
                }
            } finally {
                if (!cancelled) {
                    setHasValidatedSession(true);
                }
            }
        })();

        return () => {
            cancelled = true;
        };
    }, [clearAuth, hasHydrated, hasValidatedSession, restoreSession, setHasValidatedSession]);

    return null;
}
