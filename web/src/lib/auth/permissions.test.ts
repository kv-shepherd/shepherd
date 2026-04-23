import { describe, expect, it } from 'vitest';

import {
    ADMIN_MENU_ROUTE_PERMISSIONS,
    type AdminMenuRouteKey,
    canAccessAdminMenu,
    canAccessAdminMenuRoute,
    hasAnyPermission,
    hasPermission,
    PLATFORM_ADMIN_PERMISSION,
    USER_DIRECTORY_ROUTE_PERMISSIONS,
} from './permissions';

describe('permission helpers', () => {
  it('grants explicit permission and platform-admin override', () => {
    const user = {
      id: 'u-1',
      username: 'alice',
      permissions: ['vm:read', 'vm:create'],
    };
    expect(hasPermission(user, 'vm:create')).toBe(true);
    expect(hasPermission(user, 'system:delete')).toBe(false);

    const admin = {
      id: 'u-2',
      username: 'root',
      permissions: [PLATFORM_ADMIN_PERMISSION],
    };
    expect(hasPermission(admin, 'system:delete')).toBe(true);
    expect(hasPermission(admin, 'rbac:manage')).toBe(true);
  });

  it('evaluates any-of permission checks', () => {
    const user = {
      id: 'u-3',
      username: 'bob',
      permissions: ['service:read'],
    };
    expect(hasAnyPermission(user, ['vm:create', 'service:read'])).toBe(true);
    expect(hasAnyPermission(user, ['vm:create', 'vm:delete'])).toBe(false);
    expect(hasAnyPermission(null, ['vm:create'])).toBe(false);
  });

  it('derives admin menu access from canonical route permissions', () => {
    const reviewer = {
      id: 'u-4',
      username: 'reviewer',
      permissions: ['builtin_approval:view'],
    };
    expect(canAccessAdminMenu(reviewer)).toBe(true);
    expect(canAccessAdminMenuRoute(reviewer, 'approvalTasks')).toBe(true);
    expect(canAccessAdminMenuRoute(reviewer, 'clusters')).toBe(false);

    const regularUser = {
      id: 'u-5',
      username: 'carol',
      permissions: ['vm:create'],
    };
    expect(canAccessAdminMenu(regularUser)).toBe(false);
  });

  it('covers every admin menu route with canonical permission checks', () => {
    for (const [routeKey, permissions] of Object.entries(ADMIN_MENU_ROUTE_PERMISSIONS) as Array<
      [AdminMenuRouteKey, readonly string[]]
    >) {
      const scopedUser = {
        id: `user-${routeKey}`,
        username: routeKey,
        permissions: [permissions[0]],
      };
      expect(canAccessAdminMenu(scopedUser)).toBe(true);
      expect(canAccessAdminMenuRoute(scopedUser, routeKey)).toBe(true);

      const platformAdmin = {
        id: `admin-${routeKey}`,
        username: `admin-${routeKey}`,
        permissions: [PLATFORM_ADMIN_PERMISSION],
      };
      expect(canAccessAdminMenuRoute(platformAdmin, routeKey)).toBe(true);
    }
  });

  it('keeps the users admin route aligned with backend read permissions', () => {
    expect(ADMIN_MENU_ROUTE_PERMISSIONS.users).toEqual(USER_DIRECTORY_ROUTE_PERMISSIONS);

    const auditor = {
      id: 'u-6',
      username: 'auditor',
      permissions: ['rbac:read'],
    };

    expect(canAccessAdminMenuRoute(auditor, 'users')).toBe(true);
  });

  it('keeps the namespaces admin route aligned with namespace-scoped permissions', () => {
    expect(ADMIN_MENU_ROUTE_PERMISSIONS.namespaces).toEqual(['namespace:read', 'namespace:write']);

    const namespaceAuditor = {
      id: 'u-7',
      username: 'namespace-auditor',
      permissions: ['namespace:read'],
    };

    expect(canAccessAdminMenuRoute(namespaceAuditor, 'namespaces')).toBe(true);
    expect(canAccessAdminMenuRoute(namespaceAuditor, 'clusters')).toBe(false);
  });
});
