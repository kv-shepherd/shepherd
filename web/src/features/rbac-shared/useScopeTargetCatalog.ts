'use client';

import { useMemo } from 'react';

import { useApiGet } from '@/hooks/useApiQuery';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api.gen';

type SystemList = components['schemas']['SystemList'];
type ServiceList = components['schemas']['ServiceList'];
type VMList = components['schemas']['VMList'];

export interface ScopeTargetOption {
    value: string;
    label: string;
}

export function useScopeTargetCatalog(enabled: boolean) {
    const systemsQuery = useApiGet<SystemList>(
        ['scope-target-catalog-systems'],
        () => api.GET('/systems', {
            params: { query: { page: 1, per_page: 100 } },
        }),
        { enabled }
    );

    const servicesQuery = useApiGet<ServiceList>(
        ['scope-target-catalog-services'],
        () => api.GET('/services', {
            params: { query: { page: 1, per_page: 100 } },
        }),
        { enabled }
    );

    const vmsQuery = useApiGet<VMList>(
        ['scope-target-catalog-vms'],
        () => api.GET('/vms', {
            params: { query: { page: 1, per_page: 100 } },
        }),
        { enabled }
    );

    const scopeTargetOptionsByType = useMemo<Record<string, ScopeTargetOption[]>>(
        () => ({
            system: (systemsQuery.data?.items ?? []).map((system) => ({
                value: system.id,
                label: system.name,
            })),
            service: (servicesQuery.data?.items ?? []).map((service) => ({
                value: service.id,
                label: `${service.system_name} / ${service.name}`,
            })),
            vm: (vmsQuery.data?.items ?? []).map((vm) => ({
                value: vm.id,
                label: `${vm.name} (${vm.namespace})`,
            })),
        }),
        [servicesQuery.data?.items, systemsQuery.data?.items, vmsQuery.data?.items]
    );

    const scopeTargetLoadingByType = useMemo<Record<string, boolean>>(
        () => ({
            system: systemsQuery.isLoading,
            service: servicesQuery.isLoading,
            vm: vmsQuery.isLoading,
        }),
        [servicesQuery.isLoading, systemsQuery.isLoading, vmsQuery.isLoading]
    );

    return {
        scopeTargetOptionsByType,
        scopeTargetLoadingByType,
    };
}
