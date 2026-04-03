import type { components } from '@/types/api.gen';

export type Role = components['schemas']['Role'];
export type RoleList = components['schemas']['RoleList'];
export type RoleCreateRequest = components['schemas']['RoleCreateRequest'];
export type RoleUpdateRequest = components['schemas']['RoleUpdateRequest'];
export type Permission = components['schemas']['Permission'];
export type PermissionList = components['schemas']['PermissionList'];
export type User = components['schemas']['User'];
export type UserList = components['schemas']['UserList'];
export type GlobalRoleBinding = components['schemas']['GlobalRoleBinding'];

export const RBAC_SCOPE_VALUES = ['global', 'system', 'service', 'vm'] as const;

export const ENVIRONMENT_VALUES = ['test', 'prod'] as const;
