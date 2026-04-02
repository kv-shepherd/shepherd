import { describe, expect, it } from 'vitest';

import type { InstanceSize } from '../types';
import {
    filterAdminInstanceSizes,
    matchesInstanceSizeCapabilityFilter,
    resolveInstanceSizePublicationState,
} from './useAdminInstanceSizesController';

function buildInstanceSize(overrides: Partial<InstanceSize> = {}): InstanceSize {
    return {
        id: 'size-linux-gpu',
        name: 'gpu-workstation',
        display_name: 'GPU Workstation',
        description: 'Linux graphics workstation',
        catalog_scope: 'test',
        cpu_cores: 8,
        memory_gi: 32,
        disk_gb: 120,
        cpu_request: 6,
        memory_request_gi: 24,
        dedicated_cpu: false,
        requires_gpu: true,
        requires_hugepages: false,
        requires_sriov: false,
        enabled: true,
        spec_overrides: {},
        dv_access_modes: [],
        dv_volume_mode: undefined,
        sort_order: 0,
        created_at: '2026-04-01T00:00:00Z',
        ...overrides,
    };
}

describe('useAdminInstanceSizesController helpers', () => {
    it('resolves publication state from enabled flag and catalog scope', () => {
        expect(resolveInstanceSizePublicationState(buildInstanceSize())).toBe('ready');
        expect(
            resolveInstanceSizePublicationState(
                buildInstanceSize({ catalog_scope: 'unclassified' }),
            ),
        ).toBe('hidden');
        expect(
            resolveInstanceSizePublicationState(buildInstanceSize({ enabled: false })),
        ).toBe('disabled');
    });

    it('matches exact capability filters without exposing ids', () => {
        const gpuSize = buildInstanceSize();
        const sriovSize = buildInstanceSize({
            id: 'size-sriov',
            name: 'network-edge',
            requires_gpu: false,
            requires_sriov: true,
            cpu_request: undefined,
            memory_request_gi: undefined,
        });

        expect(matchesInstanceSizeCapabilityFilter(gpuSize, 'gpu')).toBe(true);
        expect(matchesInstanceSizeCapabilityFilter(gpuSize, 'cpu_overcommit')).toBe(true);
        expect(matchesInstanceSizeCapabilityFilter(sriovSize, 'gpu')).toBe(false);
        expect(matchesInstanceSizeCapabilityFilter(sriovSize, 'sriov')).toBe(true);
    });

    it('filters with quick search by id while keeping advanced filters name-first', () => {
        const items = [
            buildInstanceSize(),
            buildInstanceSize({
                id: 'size-prod-dedicated',
                name: 'prod-dedicated',
                display_name: 'Production Dedicated',
                description: 'Dedicated production profile',
                catalog_scope: 'prod',
                cpu_request: undefined,
                memory_request_gi: undefined,
                dedicated_cpu: true,
                requires_gpu: false,
            }),
        ];

        expect(
            filterAdminInstanceSizes(items, {
                search: 'size-prod-dedicated',
                catalogScope: '',
                enabled: '',
                publication: '',
                capability: '',
            }),
        ).toHaveLength(1);

        const filtered = filterAdminInstanceSizes(items, {
            search: '',
            catalogScope: 'prod',
            enabled: 'enabled',
            publication: 'ready',
            capability: 'dedicated_cpu',
        });

        expect(filtered).toHaveLength(1);
        expect(filtered[0].name).toBe('prod-dedicated');
    });
});
