import type { components } from '@/types/api.gen';

type UserInfo = components['schemas']['UserInfo'];

export const PLATFORM_ADMIN_PERMISSION = 'platform:admin';

export function hasPermission(user: UserInfo | null | undefined, permission: string): boolean {
    if (!user) {
        return false;
    }
    const permissions = user.permissions ?? [];
    return permissions.includes(PLATFORM_ADMIN_PERMISSION) || permissions.includes(permission);
}

export function hasAnyPermission(user: UserInfo | null | undefined, permissions: readonly string[]): boolean {
    if (!user || permissions.length === 0) {
        return false;
    }
    return permissions.some((permission) => hasPermission(user, permission));
}

