import type { components } from '@/types/api.gen';

export type PendingAdoption = components['schemas']['PendingAdoption'];
export type PendingAdoptionList = components['schemas']['PendingAdoptionList'];
export type PendingAdoptionStatus = components['schemas']['PendingAdoptionStatus'];
export type PendingAdoptionAdoptResponse = components['schemas']['PendingAdoptionAdoptResponse'];

type PendingAdoptionResourceType = components['schemas']['PendingAdoptionResourceType'];

export const PENDING_ADOPTION_PAGE_SIZE = 20;

export const PENDING_ADOPTION_STATUS_OPTIONS: PendingAdoptionStatus[] = [
    'PENDING',
    'ADOPTED',
    'REJECTED',
    'EXPIRED',
];

export const PENDING_ADOPTION_STATUS_COLORS: Record<PendingAdoptionStatus, string> = {
    PENDING: 'gold',
    ADOPTED: 'green',
    REJECTED: 'red',
    EXPIRED: 'default',
};

export const PENDING_ADOPTION_RESOURCE_TYPE: PendingAdoptionResourceType = 'VirtualMachine';
