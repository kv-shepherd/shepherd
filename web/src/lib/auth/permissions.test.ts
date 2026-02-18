import { describe, expect, it } from 'vitest';

import { hasAnyPermission, hasPermission, PLATFORM_ADMIN_PERMISSION } from './permissions';

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
});

