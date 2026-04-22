'use client';

import { useMemo } from 'react';

import { useApiGet } from '@/hooks/useApiQuery';
import { hasPermission } from '@/lib/auth/permissions';
import { api } from '@/lib/api/client';
import { useAuthStore } from '@/stores/auth';

import type { VMRequestContext } from '@/features/vm-management/types';
import type { components } from '@/types/api.gen';

type SystemList = components['schemas']['SystemList'];
type ServiceList = components['schemas']['ServiceList'];
type VMList = components['schemas']['VMList'];

interface SetupGuideSnapshot {
    systemsTotal?: number;
    servicesTotal?: number;
    vmsTotal?: number;
}

export interface SetupGuideState {
    systemsTotal: number;
    servicesTotal: number;
    vmsTotal: number;
    namespacesTotal: number;
    templatesTotal: number;
    instanceSizesTotal: number;
    canCreateSystem: boolean;
    canCreateService: boolean;
    canCreateVM: boolean;
    canManageNamespaces: boolean;
    canManageTemplates: boolean;
    canManageInstanceSizes: boolean;
    systemReady: boolean;
    serviceReady: boolean;
    prerequisitesReady: boolean;
    vmRequestReady: boolean;
    hasRequestedFirstVM: boolean;
    isLoading: boolean;
}

export function useSetupGuide(snapshot: SetupGuideSnapshot = {}): SetupGuideState {
    const user = useAuthStore((state) => state.user);

    const canCreateSystem = hasPermission(user, 'system:write');
    const canCreateService = hasPermission(user, 'service:create');
    const canCreateVM = hasPermission(user, 'vm:create');
    const canReadVM = hasPermission(user, 'vm:read');
    const canManageNamespaces = hasPermission(user, 'cluster:write');
    const canManageTemplates = hasPermission(user, 'template:write');
    const canManageInstanceSizes = hasPermission(user, 'instance_size:write');

    const systemsQuery = useApiGet<SystemList>(
        ['setup-guide', 'systems-count'],
        () => api.GET('/systems', { params: { query: { per_page: 1 } } }),
        { enabled: snapshot.systemsTotal === undefined },
    );

    const servicesQuery = useApiGet<ServiceList>(
        ['setup-guide', 'services-count'],
        () => api.GET('/services', { params: { query: { per_page: 1 } } }),
        { enabled: snapshot.servicesTotal === undefined },
    );

    const vmsQuery = useApiGet<VMList>(
        ['setup-guide', 'vms-count'],
        () => api.GET('/vms', { params: { query: { per_page: 1 } } }),
        { enabled: snapshot.vmsTotal === undefined && (canCreateVM || canReadVM) },
    );

    const requestContextQuery = useApiGet<VMRequestContext>(
        ['setup-guide', 'vm-request-context'],
        () => api.GET('/vms/request-context'),
        { enabled: canCreateVM },
    );

    const systemsTotal = snapshot.systemsTotal ?? systemsQuery.data?.pagination?.total ?? 0;
    const servicesTotal = snapshot.servicesTotal ?? servicesQuery.data?.pagination?.total ?? 0;
    const vmsTotal = snapshot.vmsTotal ?? vmsQuery.data?.pagination?.total ?? 0;

    const namespacesTotal = requestContextQuery.data?.namespaces?.length ?? 0;
    const templatesTotal = requestContextQuery.data?.templates?.length ?? 0;
    const instanceSizesTotal = requestContextQuery.data?.instance_sizes?.length ?? 0;

    return useMemo(() => {
        const systemReady = systemsTotal > 0;
        const serviceReady = servicesTotal > 0;
        const prerequisitesReady =
            namespacesTotal > 0 &&
            templatesTotal > 0 &&
            instanceSizesTotal > 0;
        const vmRequestReady = systemReady && serviceReady && prerequisitesReady;

        return {
            systemsTotal,
            servicesTotal,
            vmsTotal,
            namespacesTotal,
            templatesTotal,
            instanceSizesTotal,
            canCreateSystem,
            canCreateService,
            canCreateVM,
            canManageNamespaces,
            canManageTemplates,
            canManageInstanceSizes,
            systemReady,
            serviceReady,
            prerequisitesReady,
            vmRequestReady,
            hasRequestedFirstVM: vmsTotal > 0,
            isLoading:
                systemsQuery.isLoading ||
                servicesQuery.isLoading ||
                vmsQuery.isLoading ||
                requestContextQuery.isLoading,
        };
    }, [
        canManageInstanceSizes,
        canCreateService,
        canCreateSystem,
        canCreateVM,
        canManageNamespaces,
        canManageTemplates,
        instanceSizesTotal,
        namespacesTotal,
        requestContextQuery.isLoading,
        servicesQuery.isLoading,
        servicesTotal,
        systemsQuery.isLoading,
        systemsTotal,
        templatesTotal,
        vmsQuery.isLoading,
        vmsTotal,
    ]);
}
