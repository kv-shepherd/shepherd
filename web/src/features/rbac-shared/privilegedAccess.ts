import type { components } from '@/types/api.gen';

type RoleLike = Pick<components['schemas']['Role'], 'id' | 'permissions'>;
type RoleBindingLike = Pick<components['schemas']['GlobalRoleBinding'], 'role_id'>;

const PRIVILEGED_PERMISSION_KEYS = new Set([
    'platform:admin',
    'ticket:view',
]);

const PRIVILEGED_PERMISSION_PREFIXES = [
    'audit:',
    'auth_provider:',
    'builtin_approval:',
    'cluster:',
    'instance_size:',
    'rate_limit:',
    'rbac:',
    'template:',
    'user:',
] as const;

function isPrivilegedPermissionKey(permissionKey: string) {
    if (PRIVILEGED_PERMISSION_KEYS.has(permissionKey)) {
        return true;
    }
    return PRIVILEGED_PERMISSION_PREFIXES.some((prefix) => permissionKey.startsWith(prefix));
}

export function isPrivilegedRole(role: RoleLike) {
    return (role.permissions ?? []).some((permissionKey) => isPrivilegedPermissionKey(permissionKey));
}

export function isPrivilegedRoleBinding(
    binding: RoleBindingLike,
    rolesById: Map<string, RoleLike>,
) {
    const role = rolesById.get(binding.role_id);
    return role ? isPrivilegedRole(role) : false;
}

export function getRoleAccessTagColor(role: RoleLike | null | undefined) {
    return role && isPrivilegedRole(role) ? 'purple' : 'blue';
}
