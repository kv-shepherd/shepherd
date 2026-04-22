import type { components } from '@/types/api.gen';

type UserInfo = components['schemas']['UserInfo'];
export type AdminMenuRouteKey =
    | 'approvalTasks'
    | 'clusters'
    | 'namespaces'
    | 'templates'
    | 'instanceSizes'
    | 'users'
    | 'rbac'
    | 'rateLimits'
    | 'authProviders'
    | 'audit';

export const PLATFORM_ADMIN_PERMISSION = 'platform:admin';
export const USER_DIRECTORY_ROUTE_PERMISSIONS = ['user:manage', 'rbac:read', 'rbac:manage'] as const;
export const ADMIN_MENU_ROUTE_PERMISSIONS: Record<AdminMenuRouteKey, readonly string[]> = {
    approvalTasks: ['builtin_approval:view', 'builtin_approval:approve'],
    clusters: ['cluster:read', 'cluster:write'],
    namespaces: ['cluster:read', 'cluster:write'],
    templates: ['template:read', 'template:write'],
    instanceSizes: ['instance_size:read', 'instance_size:write'],
    users: USER_DIRECTORY_ROUTE_PERMISSIONS,
    rbac: ['rbac:read', 'rbac:manage'],
    rateLimits: ['rate_limit:manage'],
    authProviders: [
        'auth_provider:read',
        'auth_provider:configure',
        'auth_provider:update',
        'auth_provider:delete',
        'auth_provider:sync',
        'auth_provider:mapping_create',
        'auth_provider:mapping_update',
        'auth_provider:mapping_delete',
    ],
    audit: ['audit:read'],
};

const uniqueAdminMenuPermissions = Array.from(
    new Set(Object.values(ADMIN_MENU_ROUTE_PERMISSIONS).flat()),
);

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

export function canAccessAdminMenu(user: UserInfo | null | undefined): boolean {
    return hasAnyPermission(user, uniqueAdminMenuPermissions);
}

export function canAccessAdminMenuRoute(
    user: UserInfo | null | undefined,
    routeKey: AdminMenuRouteKey,
): boolean {
    return hasAnyPermission(user, ADMIN_MENU_ROUTE_PERMISSIONS[routeKey]);
}
