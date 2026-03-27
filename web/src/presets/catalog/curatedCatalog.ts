import type {
    TemplateCreateRequest,
    TemplateUpdateRequest,
} from '@/features/admin-templates/types';

import type {
    InstanceSizePresetCatalogItem,
    TemplatePresetCatalogItem,
} from './types';

export type CuratedTemplatePresetKey =
    | 'linux-test'
    | 'linux-prod'
    | 'windows-test'
    | 'windows-prod';

export type CuratedInstanceSizePresetKey = CuratedTemplatePresetKey;

export type TemplatePresetFormValues = Partial<TemplateCreateRequest & TemplateUpdateRequest>;

export interface CuratedInstanceSizePresetFormValues {
    catalog_scope: 'test' | 'prod' | 'all' | 'unclassified';
    cpu_cores: number;
    memory_gi: number;
    disk_gb: number;
    cpu_overcommit_enabled: boolean;
    cpu_request?: number;
    memory_overcommit_enabled: boolean;
    memory_request_gi?: number;
    dedicated_cpu: boolean;
    requires_sriov: boolean;
    spec_text: string;
    enabled: boolean;
}

export const curatedLinuxCloudInitExample = `#cloud-config
ssh_pwauth: true
disable_root: false
timezone: Asia/Shanghai
users:
  - name: root
    lock_passwd: false
  - name: appops
    lock_passwd: false
runcmd:
  - mkdir -p /app
  - chown -R appops:appops /app
`;

export const curatedWindowsCloudInitExample = `#ps1_sysnative
$free = (Get-Disk 0).Size - ((Get-Partition -DiskId 0 | Measure Size -Sum).Sum)
if ($free -gt 1GB) {
  $p = New-Partition -DiskNumber 0 -UseMaximumSize -DriveLetter D
  while (-not (Get-Volume -DriveLetter D -EA 0)) { sleep 1 }
  Format-Volume -DriveLetter D -FileSystem NTFS -NewFileSystemLabel "DataDisk" -Confirm:$false
}
`;

function toSpecText(spec: Record<string, unknown>): string {
    return JSON.stringify(spec, null, 2);
}

const linuxBaseSpec = {
    spec: {
        runStrategy: 'Always',
        template: {
            metadata: {
                annotations: {
                    'ovn.kubernetes.io/allow_live_migration': 'true',
                    'kubevirt.io/allow-pod-bridge-network-live-migration': 'true',
                },
            },
            spec: {
                evictionStrategy: 'LiveMigrate',
                domain: {
                    cpu: {
                        model: 'host-passthrough',
                    },
                    devices: {
                        autoattachSerialConsole: true,
                        autoattachMemBalloon: false,
                        autoattachGraphicsDevice: true,
                        autoattachVSOCK: true,
                        networkInterfaceMultiqueue: true,
                        blockMultiQueue: true,
                        rng: {},
                        disks: [
                            {
                                name: 'rootfs',
                                disk: { bus: 'virtio' },
                            },
                            {
                                name: 'cloudinitdisk',
                                disk: { bus: 'virtio' },
                            },
                        ],
                        interfaces: [
                            {
                                bridge: {},
                                name: 'default',
                                model: 'virtio',
                            },
                        ],
                    },
                    machine: {
                        type: 'q35',
                    },
                },
                livenessProbe: {
                    guestAgentPing: {},
                    initialDelaySeconds: 120,
                    periodSeconds: 20,
                    failureThreshold: 3,
                    timeoutSeconds: 5,
                },
                terminationGracePeriodSeconds: 0,
                networks: [
                    {
                        name: 'default',
                        pod: {},
                    },
                ],
            },
        },
    },
};

const windowsBaseSpec = {
    spec: {
        runStrategy: 'Always',
        template: {
            metadata: {
                annotations: {
                    'ovn.kubernetes.io/allow_live_migration': 'true',
                    'kubevirt.io/allow-pod-bridge-network-live-migration': 'true',
                },
            },
            spec: {
                evictionStrategy: 'LiveMigrate',
                domain: {
                    cpu: {
                        model: 'host-passthrough',
                        sockets: 1,
                        threads: 1,
                    },
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
                        autoattachMemBalloon: false,
                        autoattachGraphicsDevice: true,
                        autoattachVSOCK: false,
                        networkInterfaceMultiqueue: true,
                        blockMultiQueue: true,
                        rng: {},
                        disks: [
                            {
                                name: 'rootfs',
                                bootOrder: 1,
                                disk: { bus: 'scsi' },
                            },
                            {
                                name: 'cloudinitdisk',
                                cdrom: { bus: 'sata' },
                            },
                        ],
                        interfaces: [
                            {
                                bridge: {},
                                name: 'default',
                                model: 'virtio',
                            },
                        ],
                    },
                    machine: {
                        type: 'q35',
                    },
                },
                terminationGracePeriodSeconds: 180,
                networks: [
                    {
                        name: 'default',
                        pod: {},
                    },
                ],
            },
        },
    },
};

export const CURATED_TEMPLATE_PRESET_ITEMS: Array<TemplatePresetCatalogItem<TemplatePresetFormValues>> = [
    {
        key: 'linux-test',
        sourceType: 'curated',
        verificationLevel: 'verified',
        labelKey: 'templates.preset_linux_test',
        descriptionKey: 'templates.curated_group_description',
        osFamily: 'linux',
        osVersion: 'openEuler 22.03',
        pvcNamespace: 'vm-muban',
        pvcName: 'openeuler2203-image',
        imageUrlExamples: [
            'docker://quay.io/containerdisks/fedora:latest',
            'https://example.invalid/images/openeuler-22.03.qcow2',
        ],
        values: {
            os_family: 'linux',
            os_version: 'openEuler 22.03',
            catalog_scope: 'test',
            source_type: 'cdi_pvc_clone',
            pvc_namespace: 'vm-muban',
            pvc_name: 'openeuler2203-image',
            image_url: undefined,
            cloud_init: curatedLinuxCloudInitExample,
            enabled: true,
        },
    },
    {
        key: 'linux-prod',
        sourceType: 'curated',
        verificationLevel: 'verified',
        labelKey: 'templates.preset_linux_prod',
        descriptionKey: 'templates.curated_group_description',
        osFamily: 'linux',
        osVersion: 'Kylin V10',
        pvcNamespace: 'vm-muban',
        pvcName: 'kylinv10-image',
        imageUrlExamples: [
            'docker://registry.example.com/vm-images/kylin:v10',
            'https://example.invalid/images/kylin-v10.qcow2',
        ],
        values: {
            os_family: 'linux',
            os_version: 'Kylin V10',
            catalog_scope: 'prod',
            source_type: 'cdi_pvc_clone',
            pvc_namespace: 'vm-muban',
            pvc_name: 'kylinv10-image',
            image_url: undefined,
            cloud_init: curatedLinuxCloudInitExample,
            enabled: true,
        },
    },
    {
        key: 'windows-test',
        sourceType: 'curated',
        verificationLevel: 'verified',
        labelKey: 'templates.preset_windows_test',
        descriptionKey: 'templates.curated_group_description',
        osFamily: 'windows',
        osVersion: 'Windows Server 2022',
        pvcNamespace: 'vm-muban',
        pvcName: 'win2022-image',
        imageUrlExamples: [
            'docker://registry.example.com/vm-images/windows-server-2022:latest',
            'https://example.invalid/images/windows-server-2022.qcow2',
        ],
        values: {
            os_family: 'windows',
            os_version: 'Windows Server 2022',
            catalog_scope: 'test',
            source_type: 'cdi_pvc_clone',
            pvc_namespace: 'vm-muban',
            pvc_name: 'win2022-image',
            image_url: undefined,
            cloud_init: curatedWindowsCloudInitExample,
            enabled: true,
        },
    },
    {
        key: 'windows-prod',
        sourceType: 'curated',
        verificationLevel: 'verified',
        labelKey: 'templates.preset_windows_prod',
        descriptionKey: 'templates.curated_group_description',
        osFamily: 'windows',
        osVersion: 'Windows Server 2022',
        pvcNamespace: 'vm-muban',
        pvcName: 'win2022-image',
        imageUrlExamples: [
            'docker://registry.example.com/vm-images/windows-server-2022:latest',
            'https://example.invalid/images/windows-server-2022.qcow2',
        ],
        values: {
            os_family: 'windows',
            os_version: 'Windows Server 2022',
            catalog_scope: 'prod',
            source_type: 'cdi_pvc_clone',
            pvc_namespace: 'vm-muban',
            pvc_name: 'win2022-image',
            image_url: undefined,
            cloud_init: curatedWindowsCloudInitExample,
            enabled: true,
        },
    },
];

export const CURATED_INSTANCE_SIZE_PRESET_ITEMS: Array<InstanceSizePresetCatalogItem<CuratedInstanceSizePresetFormValues>> = [
    {
        key: 'linux-test',
        sourceType: 'curated',
        verificationLevel: 'verified',
        labelKey: 'instanceSizes.preset_linux_test',
        descriptionKey: 'instanceSizes.preset_description',
        values: {
            catalog_scope: 'test',
            cpu_cores: 4,
            memory_gi: 8,
            disk_gb: 60,
            cpu_overcommit_enabled: true,
            cpu_request: 2,
            memory_overcommit_enabled: true,
            memory_request_gi: 4,
            dedicated_cpu: false,
            requires_sriov: false,
            spec_text: toSpecText({
                ...linuxBaseSpec,
                spec: linuxBaseSpec.spec,
            }),
            enabled: true,
        },
    },
    {
        key: 'linux-prod',
        sourceType: 'curated',
        verificationLevel: 'verified',
        labelKey: 'instanceSizes.preset_linux_prod',
        descriptionKey: 'instanceSizes.preset_description',
        values: {
            catalog_scope: 'prod',
            cpu_cores: 4,
            memory_gi: 8,
            disk_gb: 60,
            cpu_overcommit_enabled: false,
            cpu_request: undefined,
            memory_overcommit_enabled: false,
            memory_request_gi: undefined,
            dedicated_cpu: true,
            requires_sriov: false,
            spec_text: toSpecText({
                ...linuxBaseSpec,
                spec: {
                    ...linuxBaseSpec.spec,
                    template: {
                        ...linuxBaseSpec.spec.template,
                        spec: {
                            ...linuxBaseSpec.spec.template.spec,
                            domain: {
                                ...linuxBaseSpec.spec.template.spec.domain,
                                cpu: {
                                    ...linuxBaseSpec.spec.template.spec.domain.cpu,
                                    sockets: 1,
                                    threads: 1,
                                    dedicatedCpuPlacement: true,
                                    isolateEmulatorThread: false,
                                    numa: {
                                        guestMappingPassthrough: {},
                                    },
                                },
                                memory: {
                                    hugepages: {
                                        pageSize: '2Mi',
                                    },
                                },
                            },
                        },
                    },
                },
            }),
            enabled: true,
        },
    },
    {
        key: 'windows-test',
        sourceType: 'curated',
        verificationLevel: 'verified',
        labelKey: 'instanceSizes.preset_windows_test',
        descriptionKey: 'instanceSizes.preset_description',
        values: {
            catalog_scope: 'test',
            cpu_cores: 8,
            memory_gi: 16,
            disk_gb: 120,
            cpu_overcommit_enabled: true,
            cpu_request: 4,
            memory_overcommit_enabled: true,
            memory_request_gi: 8,
            dedicated_cpu: false,
            requires_sriov: false,
            spec_text: toSpecText({
                ...windowsBaseSpec,
                spec: windowsBaseSpec.spec,
            }),
            enabled: true,
        },
    },
    {
        key: 'windows-prod',
        sourceType: 'curated',
        verificationLevel: 'verified',
        labelKey: 'instanceSizes.preset_windows_prod',
        descriptionKey: 'instanceSizes.preset_description',
        values: {
            catalog_scope: 'prod',
            cpu_cores: 8,
            memory_gi: 16,
            disk_gb: 120,
            cpu_overcommit_enabled: false,
            cpu_request: undefined,
            memory_overcommit_enabled: false,
            memory_request_gi: undefined,
            dedicated_cpu: true,
            requires_sriov: false,
            spec_text: toSpecText({
                ...windowsBaseSpec,
                spec: {
                    ...windowsBaseSpec.spec,
                    template: {
                        ...windowsBaseSpec.spec.template,
                        spec: {
                            ...windowsBaseSpec.spec.template.spec,
                            domain: {
                                ...windowsBaseSpec.spec.template.spec.domain,
                                cpu: {
                                    ...windowsBaseSpec.spec.template.spec.domain.cpu,
                                    dedicatedCpuPlacement: true,
                                    isolateEmulatorThread: false,
                                    numa: {
                                        guestMappingPassthrough: {},
                                    },
                                },
                                memory: {
                                    hugepages: {
                                        pageSize: '2Mi',
                                    },
                                },
                            },
                        },
                    },
                },
            }),
            enabled: true,
        },
    },
];
