import type { components } from '@/types/api.gen';

export type InstanceSize = components['schemas']['InstanceSize'];
export type InstanceSizeList = components['schemas']['InstanceSizeList'];
export type InstanceSizeCreateRequest = components['schemas']['InstanceSizeCreateRequest'];
export type InstanceSizeUpdateRequest = components['schemas']['InstanceSizeUpdateRequest'];

export function formatMemory(mb: number): string {
    if (mb >= 1024) {
        const gb = mb / 1024;
        return `${Number.isInteger(gb) ? gb : gb.toFixed(1)} GB`;
    }
    return `${mb} MB`;
}

export function getCapabilityLabels(record: InstanceSize): string[] {
    const labels: string[] = [];
    if (record.requires_gpu) labels.push('GPU');
    if (record.requires_sriov) labels.push('SR-IOV');
    if (record.requires_hugepages) labels.push('Hugepages');
    if (record.dedicated_cpu) labels.push('Dedicated CPU');
    return labels;
}
