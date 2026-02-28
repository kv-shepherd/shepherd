import type { components } from '@/types/api.gen';

export type InstanceSize = components['schemas']['InstanceSize'];
export type InstanceSizeList = components['schemas']['InstanceSizeList'];
export type InstanceSizeCreateRequest = components['schemas']['InstanceSizeCreateRequest'];
export type InstanceSizeUpdateRequest = components['schemas']['InstanceSizeUpdateRequest'];

/**
 * Formats memory in GiB for display.
 * Values are already in Gi from the API (post int→float64 migration).
 * 0.5-step increments are supported (e.g. 0.5 Gi, 1 Gi, 1.5 Gi).
 */
export function formatMemory(gi: number): string {
    if (!Number.isFinite(gi) || gi <= 0) {
        return '0 Gi';
    }
    return `${Number.isInteger(gi) ? gi : gi.toFixed(1)} Gi`;
}

export function getCapabilityLabels(record: InstanceSize): string[] {
    const labels: string[] = [];
    if (record.requires_gpu) labels.push('GPU');
    if (record.requires_sriov) labels.push('SR-IOV');
    if (record.requires_hugepages) labels.push('Hugepages');
    if (record.dedicated_cpu) labels.push('Dedicated CPU');
    return labels;
}
