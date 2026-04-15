import { describe, expect, it } from 'vitest';

import { CURATED_INSTANCE_SIZE_PRESET_ITEMS } from './curatedCatalog';

function parseSpecText(specText: string): Record<string, unknown> {
    return JSON.parse(specText) as Record<string, unknown>;
}

function getAtPath(value: unknown, pathText: string): unknown {
    const parts = pathText.split('.');
    let current: unknown = value;
    for (const part of parts) {
        if (Array.isArray(current)) {
            const index = Number(part);
            if (Number.isNaN(index)) {
                return undefined;
            }
            current = current[index];
            continue;
        }
        if (!current || typeof current !== 'object') {
            return undefined;
        }
        current = (current as Record<string, unknown>)[part];
    }
    return current;
}

function getNameList(value: unknown, pathText: string): string[] {
    const items = getAtPath(value, pathText);
    if (!Array.isArray(items)) {
        return [];
    }
    return items
        .map((item) => (item && typeof item === 'object' ? (item as { name?: unknown }).name : undefined))
        .filter((item): item is string => typeof item === 'string');
}

describe('curatedCatalog internal audit', () => {
    it.each(CURATED_INSTANCE_SIZE_PRESET_ITEMS)(
        'keeps instance-size preset $key internally consistent',
        ({ key, sourceType, verificationLevel, values }) => {
            const parsed = parseSpecText(values.spec_text);

            expect(sourceType).toBe('curated');
            expect(verificationLevel).toBe('verified');
            expect(values.catalog_scope).toBe(key.includes('test') ? 'test' : 'prod');
            expect(values.enabled).toBe(true);
            expect(values.requires_sriov).toBe(false);
            expect(getAtPath(parsed, 'spec.runStrategy')).toBe('Always');

            // InstanceSize keeps capacity and overcommit data in top-level catalog fields.
            expect(getAtPath(parsed, 'spec.template.spec.domain.cpu.cores')).toBeUndefined();
            expect(getAtPath(parsed, 'spec.template.spec.domain.memory.guest')).toBeUndefined();
            expect(getAtPath(parsed, 'spec.template.spec.domain.resources')).toBeUndefined();

            expect(getNameList(parsed, 'spec.template.spec.domain.devices.disks')).toEqual(
                expect.arrayContaining(['rootfs', 'cloudinitdisk']),
            );
            expect(getNameList(parsed, 'spec.template.spec.domain.devices.interfaces')).toContain('default');
            expect(getNameList(parsed, 'spec.template.spec.networks')).toContain('default');
            expect(
                getAtPath(
                    parsed,
                    'spec.template.spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.weight',
                ),
            ).toBe(100);
            expect(
                getAtPath(
                    parsed,
                    'spec.template.spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.podAffinityTerm.labelSelector.matchExpressions.0.key',
                ),
            ).toBe('shepherd.io/service-id');
            expect(
                getAtPath(
                    parsed,
                    'spec.template.spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.podAffinityTerm.labelSelector.matchExpressions.0.operator',
                ),
            ).toBe('In');
            expect(
                getAtPath(
                    parsed,
                    'spec.template.spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.podAffinityTerm.labelSelector.matchExpressions.0.values.0',
                ),
            ).toBe('__SHEPHERD_SERVICE_ID__');
            expect(
                getAtPath(
                    parsed,
                    'spec.template.spec.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.podAffinityTerm.topologyKey',
                ),
            ).toBe('kubernetes.io/hostname');

            if (key.startsWith('linux')) {
                expect(values.cpu_cores).toBe(4);
                expect(values.memory_gi).toBe(8);
                expect(values.disk_gb).toBe(60);
            } else {
                expect(values.cpu_cores).toBe(8);
                expect(values.memory_gi).toBe(16);
                expect(values.disk_gb).toBe(120);
            }

            if (key.endsWith('test')) {
                expect(values.cpu_overcommit_enabled).toBe(true);
                expect(values.cpu_request).toBeDefined();
                expect(values.memory_overcommit_enabled).toBe(true);
                expect(values.memory_request_gi).toBeDefined();
                expect(values.dedicated_cpu).toBe(false);
                expect(getAtPath(parsed, 'spec.template.spec.domain.cpu.dedicatedCpuPlacement')).not.toBe(true);
            } else {
                expect(values.cpu_overcommit_enabled).toBe(false);
                expect(values.cpu_request).toBeUndefined();
                expect(values.memory_overcommit_enabled).toBe(false);
                expect(values.memory_request_gi).toBeUndefined();
                expect(values.dedicated_cpu).toBe(true);
                expect(getAtPath(parsed, 'spec.template.spec.domain.cpu.dedicatedCpuPlacement')).toBe(true);
                expect(getAtPath(parsed, 'spec.template.spec.domain.cpu.numa.guestMappingPassthrough')).toBeUndefined();
                expect(getAtPath(parsed, 'spec.template.spec.domain.memory.hugepages.pageSize')).toBe('2Mi');
            }
        },
    );
});
