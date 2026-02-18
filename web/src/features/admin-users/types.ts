import type { components } from '@/types/api.gen';

export type User = components['schemas']['User'];
export type UserList = components['schemas']['UserList'];
export type UserCreateRequest = components['schemas']['UserCreateRequest'];
export type UserUpdateRequest = components['schemas']['UserUpdateRequest'];
export type System = components['schemas']['System'];
export type SystemList = components['schemas']['SystemList'];
export type SystemMember = components['schemas']['SystemMember'];
export type SystemMemberList = components['schemas']['SystemMemberList'];
export type SystemMemberCreateRequest = components['schemas']['SystemMemberCreateRequest'];
export type SystemMemberRoleUpdateRequest = components['schemas']['SystemMemberRoleUpdateRequest'];
export type RateLimitExemption = components['schemas']['RateLimitExemption'];
export type RateLimitExemptionCreateRequest = components['schemas']['RateLimitExemptionCreateRequest'];
export type RateLimitUserOverride = components['schemas']['RateLimitUserOverride'];
export type RateLimitUserOverrideRequest = components['schemas']['RateLimitUserOverrideRequest'];
export type RateLimitStatusList = components['schemas']['RateLimitStatusList'];
export type RateLimitUserStatus = components['schemas']['RateLimitUserStatus'];

export const MEMBER_ROLE_VALUES: Array<NonNullable<SystemMember['role']>> = [
    'owner',
    'admin',
    'member',
    'viewer',
];
