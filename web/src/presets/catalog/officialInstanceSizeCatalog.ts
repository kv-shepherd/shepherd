import type { InstanceSizePresetCatalogItem } from './types';
import type { CuratedInstanceSizePresetFormValues } from './curatedCatalog';

export type OfficialInstanceSizePresetKey =
    | 'official-linux-general'
    | 'official-windows-general';

function toSpecText(spec: Record<string, unknown>): string {
    return JSON.stringify(spec, null, 2);
}

const graphicsDeviceOverrideSpec = {
    spec: {
        template: {
            spec: {
                domain: {
                    devices: {
                        autoattachGraphicsDevice: true,
                    },
                },
            },
        },
    },
};

// KubeVirt common-instancetypes recommends the workload-agnostic U series as a
// generic starting point. These presets keep only the broadest, lowest-risk
// Linux/Windows settings and intentionally avoid environment-specific tuning
// such as hugepages, dedicated CPU, NUMA, or storage-class assumptions.
export const OFFICIAL_INSTANCE_SIZE_PRESET_ITEMS: Array<
    InstanceSizePresetCatalogItem<CuratedInstanceSizePresetFormValues>
> = [
    {
        key: 'official-linux-general',
        sourceType: 'official',
        verificationLevel: 'verified',
        labelKey: 'instanceSizes.official_preset_linux_general',
        descriptionKey: 'instanceSizes.recommended_group_description',
        values: {
            catalog_scope: 'all',
            cpu_cores: 2,
            memory_gi: 4,
            disk_gb: 60,
            cpu_overcommit_enabled: false,
            cpu_request: undefined,
            memory_overcommit_enabled: false,
            memory_request_gi: undefined,
            dedicated_cpu: false,
            requires_sriov: false,
            system_labels: ['os:linux'],
            spec_text: toSpecText(graphicsDeviceOverrideSpec),
            enabled: true,
        },
    },
    {
        key: 'official-windows-general',
        sourceType: 'official',
        verificationLevel: 'verified',
        labelKey: 'instanceSizes.official_preset_windows_general',
        descriptionKey: 'instanceSizes.recommended_group_description',
        values: {
            catalog_scope: 'all',
            cpu_cores: 2,
            memory_gi: 8,
            disk_gb: 120,
            cpu_overcommit_enabled: false,
            cpu_request: undefined,
            memory_overcommit_enabled: false,
            memory_request_gi: undefined,
            dedicated_cpu: false,
            requires_sriov: false,
            system_labels: ['os:windows'],
            spec_text: toSpecText({
                ...graphicsDeviceOverrideSpec,
                spec: {
                    ...graphicsDeviceOverrideSpec.spec,
                    template: {
                        ...graphicsDeviceOverrideSpec.spec.template,
                        spec: {
                            ...graphicsDeviceOverrideSpec.spec.template.spec,
                            domain: {
                                ...graphicsDeviceOverrideSpec.spec.template.spec.domain,
                                clock: {
                                    utc: {},
                                    timer: {
                                        hpet: { present: false },
                                        pit: { tickPolicy: 'delay' },
                                        rtc: { tickPolicy: 'delay' },
                                        hyperv: {},
                                    },
                                },
                                features: {
                                    acpi: { enabled: true },
                                    apic: { enabled: true },
                                    hyperv: {
                                        relaxed: { enabled: true },
                                        vapic: { enabled: true },
                                        spinlocks: { enabled: true, spinlocks: 8191 },
                                        vpindex: { enabled: true },
                                        runtime: { enabled: true },
                                        synic: { enabled: true },
                                        frequencies: { enabled: true },
                                        reset: { enabled: true },
                                        vendorid: { enabled: true, vendorid: 'KubeVirt' },
                                    },
                                },
                                devices: {
                                    ...graphicsDeviceOverrideSpec.spec.template.spec.domain.devices,
                                    autoattachMemBalloon: false,
                                    autoattachVSOCK: false,
                                },
                            },
                            terminationGracePeriodSeconds: 180,
                        },
                    },
                },
            }),
            enabled: true,
        },
    },
];
