import type { components } from '@/types/api.gen';

export type NamespaceRegistry = components['schemas']['NamespaceRegistry'];
export type NamespaceRegistryList = components['schemas']['NamespaceRegistryList'];
export type NamespaceCreateRequest = components['schemas']['NamespaceCreateRequest'];
export type NamespaceUpdateRequest = components['schemas']['NamespaceUpdateRequest'];

export const ENV_MAP: Record<string, { color: string; label: string }> = {
    test: { color: 'blue', label: 'Test' },
    prod: { color: 'red', label: 'Production' },
};

export const ENV_OPTIONS = [
    { value: 'test', label: 'Test' },
    { value: 'prod', label: 'Production' },
] as const;
