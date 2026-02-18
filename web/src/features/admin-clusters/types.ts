import type { components } from '@/types/api.gen';

export type Cluster = components['schemas']['Cluster'];
export type ClusterList = components['schemas']['ClusterList'];
export type ClusterCreateRequest = components['schemas']['ClusterCreateRequest'];

export const CLUSTER_STATUS_MAP: Record<string, { color: string; badge: 'success' | 'error' | 'warning' | 'default' }> = {
    HEALTHY: { color: 'green', badge: 'success' },
    UNHEALTHY: { color: 'red', badge: 'error' },
    UNREACHABLE: { color: 'orange', badge: 'warning' },
    UNKNOWN: { color: 'default', badge: 'default' },
};
