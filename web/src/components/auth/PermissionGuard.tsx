'use client';

import type { ReactNode } from 'react';

import { useAuthStore } from '@/stores/auth';
import { hasAnyPermission, hasPermission } from '@/lib/auth/permissions';

interface PermissionGuardProps {
    children: ReactNode;
    permission?: string;
    anyOf?: readonly string[];
    fallback?: ReactNode;
}

export function PermissionGuard({ children, permission, anyOf, fallback = null }: PermissionGuardProps) {
    const user = useAuthStore((state) => state.user);

    const allowed = (() => {
        if (permission) {
            return hasPermission(user, permission);
        }
        if (anyOf && anyOf.length > 0) {
            return hasAnyPermission(user, anyOf);
        }
        return true;
    })();

    return allowed ? <>{children}</> : <>{fallback}</>;
}

