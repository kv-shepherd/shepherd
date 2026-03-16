import { describe, expect, it } from 'vitest';

import { buildInstanceSizePresetValues } from './instanceSizePresets';

describe('instanceSizePresets', () => {
    it('builds the linux test preset with linked overcommit values', () => {
        const preset = buildInstanceSizePresetValues('linux-test');
        const spec = JSON.parse(preset.spec_text) as {
            spec?: {
                template?: {
                    spec?: {
                        nodeSelector?: Record<string, string>;
                    };
                };
            };
        };

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
        expect(
            spec.spec?.template?.spec?.nodeSelector,
        ).toMatchObject({
            'kubevirt.io/ksm-enabled': 'true',
        });
    });

    it('builds the windows prod preset with dedicated cpu and hugepages spec', () => {
        const preset = buildInstanceSizePresetValues('windows-prod');
        const spec = JSON.parse(preset.spec_text) as {
            spec?: {
                template?: {
                    spec?: {
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
        expect(
            spec.spec?.template?.spec?.domain?.cpu?.dedicatedCpuPlacement,
        ).toBe(true);
        expect(
            spec.spec?.template?.spec?.domain?.memory,
        ).toMatchObject({
            hugepages: {
                pageSize: '2Mi',
            },
        });
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
});
