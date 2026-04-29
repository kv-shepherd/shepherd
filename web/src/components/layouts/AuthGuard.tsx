'use client';

/**
 * Auth guard component.
 *
 * Wraps protected routes — redirects to /login if unauthenticated.
 * Handles force password change flow (master-flow Stage 1.5).
 *
 * AGENTS.md §8.1: Initialize once, not per mount.
 */
import { useEffect } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { Spin } from 'antd';
import { useAuthStore } from '@/stores/auth';
import { consumeNextLoginEntry } from '@/lib/auth/loginEntry';

export default function AuthGuard({
    children,
}: {
    children: React.ReactNode;
}) {
    const router = useRouter();
    const pathname = usePathname();
    const { isAuthenticated, forcePasswordChange, hasHydrated, hasValidatedSession } = useAuthStore();

    useEffect(() => {
        if (!hasHydrated || !hasValidatedSession) {
            return;
        }

        if (!isAuthenticated) {
            router.replace(consumeNextLoginEntry());
            return;
        }

        if (forcePasswordChange && pathname !== '/auth/change-password') {
            router.replace('/auth/change-password');
        }
    }, [forcePasswordChange, hasHydrated, hasValidatedSession, isAuthenticated, pathname, router]);

    if (!hasHydrated || !hasValidatedSession || !isAuthenticated) {
        return (
            <div
                style={{
                    display: 'flex',
                    justifyContent: 'center',
                    alignItems: 'center',
                    minHeight: '100vh',
                }}
            >
                <Spin size="large" />
            </div>
        );
    }

    return <>{children}</>;
}
