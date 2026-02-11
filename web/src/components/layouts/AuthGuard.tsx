'use client';

/**
 * Auth guard component.
 *
 * Wraps protected routes — redirects to /login if unauthenticated.
 * Handles force password change flow (master-flow Stage 1.5).
 *
 * AGENTS.md §8.1: Initialize once, not per mount.
 */
import { useEffect, useRef } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { Spin } from 'antd';
import { useAuthStore } from '@/stores/auth';

export default function AuthGuard({
    children,
}: {
    children: React.ReactNode;
}) {
    const router = useRouter();
    const pathname = usePathname();
    const { isAuthenticated, forcePasswordChange } = useAuthStore();
    const checkedRef = useRef(false);

    useEffect(() => {
        if (checkedRef.current) return;
        checkedRef.current = true;

        if (!isAuthenticated) {
            router.replace('/login');
            return;
        }

        if (forcePasswordChange && pathname !== '/auth/change-password') {
            router.replace('/auth/change-password');
        }
    }, [isAuthenticated, forcePasswordChange, pathname, router]);

    if (!isAuthenticated) {
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
