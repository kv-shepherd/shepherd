import type { components } from '@/types/api.gen';

export type System = components['schemas']['System'];
export type SystemList = components['schemas']['SystemList'];
export type SystemFilterOptions = components['schemas']['SystemFilterOptionsResponse'];
export type SystemCreateRequest = components['schemas']['SystemCreateRequest'];
export type SystemUpdateRequest = components['schemas']['SystemUpdateRequest'];
export type SystemMember = components['schemas']['SystemMember'];
export type SystemMemberList = components['schemas']['SystemMemberList'];
export type SystemMemberRoleUpdateRequest = components['schemas']['SystemMemberRoleUpdateRequest'];
export type UserList = components['schemas']['UserList'];

/** RFC 1035 label validation */
export const RFC1035_PATTERN = /^[a-z]([a-z0-9-]*[a-z0-9])?$/;
