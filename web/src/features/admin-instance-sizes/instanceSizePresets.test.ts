import { describe, expect, it } from 'vitest';

import { buildInstanceSizePresetValues, getInstanceSizePresetGroups } from './instanceSizePresets';

describe('instanceSizePresets', () => {
    it('builds the linux test preset with linked overcommit values', () => {
        const preset = buildInstanceSizePresetValues('linux-test');

        expect(preset).toMatchObject({
            catalog_scope: 'test',
            cpu_cores: 4,
            memory_gi: 8,
            disk_gb: 60,
            cpu_overcommit_enabled: true,
            cpu_request: 2,
            memory_overcommit_enabled: true,
            memory_request_gi: 4,
            dedicated_cpu: false,
            enabled: true,
        });
    });

    it('builds the windows prod preset with dedicated cpu and hugepages spec', () => {
        const preset = buildInstanceSizePresetValues('windows-prod');
        const spec = JSON.parse(preset.spec_text) as {
            spec?: {
                template?: {
                    spec?: {
                        affinity?: {
                            podAntiAffinity?: {
                                requiredDuringSchedulingIgnoredDuringExecution?: Array<{
                                    labelSelector?: {
                                        matchExpressions?: Array<{
                                            key?: string;
                                            operator?: string;
                                            values?: string[];
                                        }>;
                                    };
                                    topologyKey?: string;
                                }>;
                            };
                        };
                        domain?: {
                            cpu?: Record<string, unknown>;
                            memory?: Record<string, unknown>;
                            features?: {
                                hyperv?: {
                                    vendorid?: Record<string, unknown>;
                                    spinlocks?: Record<string, unknown>;
                                };
                            };
                            clock?: Record<string, unknown>;
                        };
                    };
                };
            };
        };

        expect(preset.catalog_scope).toBe('prod');
        expect(preset.dedicated_cpu).toBe(true);
        expect(preset.cpu_overcommit_enabled).toBe(false);
        // dedicatedCpuPlacement intentionally absent: the preset relies solely
        // on the indexed `dedicated_cpu: true` column (ADR-0018 §4 / ADR-0036).
        // The form would strip it via stripIndexedSpecOverridePaths anyway.
        expect(
            spec.spec?.template?.spec?.domain?.cpu?.dedicatedCpuPlacement,
        ).toBeUndefined();
        expect(
            spec.spec?.template?.spec?.domain?.cpu?.numa,
        ).toBeUndefined();
        expect(
            spec.spec?.template?.spec?.domain?.memory,
        ).toMatchObject({
            hugepages: {
                pageSize: '2Mi',
            },
        });
        expect(
            spec.spec?.template?.spec?.affinity?.podAntiAffinity?.requiredDuringSchedulingIgnoredDuringExecution,
        ).toEqual([
            {
                labelSelector: {
                    matchExpressions: [
                        {
                            key: 'shepherd.io/service-id',
                            operator: 'In',
                            values: ['__SHEPHERD_SERVICE_ID__'],
                        },
                    ],
                },
                topologyKey: 'kubernetes.io/hostname',
            },
        ]);
        expect(
            spec.spec?.template?.spec?.domain?.features?.hyperv?.vendorid,
        ).toMatchObject({
            enabled: true,
            vendorid: 'KubeVirt',
        });
        expect(
            spec.spec?.template?.spec?.domain?.features?.hyperv?.spinlocks,
        ).toMatchObject({
            enabled: true,
            spinlocks: 8191,
        });
        expect(
            spec.spec?.template?.spec?.domain?.clock,
        ).toMatchObject({
            utc: {},
            timer: {
                hpet: { present: false },
                pit: { tickPolicy: 'delay' },
                rtc: { tickPolicy: 'delay' },
                hyperv: {},
            },
        });
    });

    it('groups customized presets into test and prod sections', () => {
        const groups = getInstanceSizePresetGroups();
        expect(groups).toHaveLength(2);
        expect(groups[0]?.sourceType).toBe('official');
        expect(groups[0]?.scopeGroups.map((group) => group.scope)).toEqual(['all']);
        expect(groups[1]?.sourceType).toBe('curated');
        expect(groups[1]?.scopeGroups.map((group) => group.scope)).toEqual(['test', 'prod']);
    });

    it('builds the community linux and windows baseline presets', () => {
        expect(buildInstanceSizePresetValues('official-linux-general')).toMatchObject({
            catalog_scope: 'all',
            cpu_cores: 2,
            memory_gi: 4,
            disk_gb: 60,
            dedicated_cpu: false,
            enabled: true,
        });

        const windowsPreset = buildInstanceSizePresetValues('official-windows-general');
        const spec = JSON.parse(windowsPreset.spec_text) as {
            spec?: {
                template?: {
                    spec?: {
                        domain?: {
                            features?: Record<string, unknown>;
                            clock?: Record<string, unknown>;
                        };
                    };
                };
            };
        };

        expect(windowsPreset).toMatchObject({
            catalog_scope: 'all',
            cpu_cores: 2,
            memory_gi: 8,
            disk_gb: 120,
            dedicated_cpu: false,
            enabled: true,
        });
        expect(spec.spec?.template?.spec?.domain?.features).toBeTruthy();
        expect(spec.spec?.template?.spec?.domain?.clock).toBeTruthy();
    });

    it('adds anti-affinity to curated linux test preset without injecting numa passthrough', () => {
        const preset = buildInstanceSizePresetValues('linux-test');
        const spec = JSON.parse(preset.spec_text) as {
            spec?: {
                template?: {
                    spec?: {
                        affinity?: {
                            podAntiAffinity?: {
                                requiredDuringSchedulingIgnoredDuringExecution?: Array<{
                                    labelSelector?: {
                                        matchExpressions?: Array<{
                                            key?: string;
                                            operator?: string;
                                            values?: string[];
                                        }>;
                                    };
                                    topologyKey?: string;
                                }>;
                            };
                        };
                        domain?: {
                            cpu?: Record<string, unknown>;
                        };
                    };
                };
            };
        };

        expect(
            spec.spec?.template?.spec?.affinity?.podAntiAffinity?.requiredDuringSchedulingIgnoredDuringExecution,
        ).toEqual([
            {
                labelSelector: {
                    matchExpressions: [
                        {
                            key: 'shepherd.io/service-id',
                            operator: 'In',
                            values: ['__SHEPHERD_SERVICE_ID__'],
                        },
                    ],
                },
                topologyKey: 'kubernetes.io/hostname',
            },
        ]);
        expect(spec.spec?.template?.spec?.domain?.cpu?.numa).toBeUndefined();
    });
});
