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

export function formatCores(cores: number): string {
    if (!Number.isFinite(cores) || cores <= 0) {
        return '0';
    }
    return Number.isInteger(cores) ? `${cores}` : cores.toFixed(1);
}

function getSpecOverrideValue(spec: Record<string, unknown> | undefined, path: string): unknown {
    if (!spec || !path) {
        return undefined;
    }
    if (path in spec) {
        return spec[path];
    }
    const segments = path.split('.');
    let current: unknown = spec;
    for (const segment of segments) {
        if (!current || typeof current !== 'object' || Array.isArray(current)) {
            return undefined;
        }
        current = (current as Record<string, unknown>)[segment];
    }
    return current;
}

export function hasCPUOvercommit(record: InstanceSize): boolean {
    return typeof record.cpu_request === 'number' && record.cpu_request > 0 && record.cpu_request < record.cpu_cores;
}

export function hasMemoryOvercommit(record: InstanceSize): boolean {
    return typeof record.memory_request_gi === 'number' && record.memory_request_gi > 0 && record.memory_request_gi < record.memory_gi;
}

export function getGPUDeviceLabels(record: InstanceSize): string[] {
    const raw = getSpecOverrideValue(
        record.spec_overrides as Record<string, unknown> | undefined,
        'spec.template.spec.domain.devices.gpus'
    );
    if (!Array.isArray(raw)) {
        return [];
    }
    const labels = raw
        .map((item) => {
            if (!item || typeof item !== 'object' || Array.isArray(item)) {
                return '';
            }
            const typed = item as Record<string, unknown>;
            const deviceName = typeof typed.deviceName === 'string' ? typed.deviceName.trim() : '';
            const name = typeof typed.name === 'string' ? typed.name.trim() : '';
            return deviceName || name;
        })
        .filter((value): value is string => value.length > 0);
    return Array.from(new Set(labels));
}

export function getCapabilityLabels(record: InstanceSize): string[] {
    const labels: string[] = [];
    if (hasCPUOvercommit(record)) labels.push(`CPU Overcommit ${formatCores(record.cpu_request!)}/${formatCores(record.cpu_cores)}`);
    if (hasMemoryOvercommit(record)) labels.push(`Memory Overcommit ${formatMemory(record.memory_request_gi!)}/${formatMemory(record.memory_gi)}`);
    const gpuDevices = getGPUDeviceLabels(record);
    if (gpuDevices.length > 0) {
        labels.push(...gpuDevices.map((device) => `GPU ${device}`));
    } else if (record.requires_gpu) {
        labels.push('GPU');
    }
    if (record.requires_sriov) labels.push('SR-IOV');
    if (record.requires_hugepages) labels.push('Hugepages');
    if (record.dedicated_cpu) labels.push('Dedicated CPU');
    return labels;
}
