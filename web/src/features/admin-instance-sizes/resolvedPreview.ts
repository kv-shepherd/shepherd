export interface InstanceSizeResolvedPreviewValues {
    catalog_scope?: string;
    cpu_cores?: number;
    memory_gi?: number;
    disk_gb?: number;
    cpu_request?: number;
    memory_request_gi?: number;
    cpu_overcommit_enabled?: boolean;
    memory_overcommit_enabled?: boolean;
    dedicated_cpu?: boolean;
    requires_sriov?: boolean;
    root_volume_mode_intent?: 'auto' | 'explicit';
    dv_access_modes?: string[];
    dv_volume_mode?: 'Block' | 'Filesystem';
    spec_text?: string;
    enabled?: boolean;
}

function cloneObject<T>(value: T): T {
    return JSON.parse(JSON.stringify(value)) as T;
}

function parseSpecText(specText?: string): Record<string, unknown> {
    if (!specText || !specText.trim() || specText.trim() === '{}') {
        return {};
    }

    try {
        const parsed = JSON.parse(specText) as unknown;
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
            return cloneObject(parsed as Record<string, unknown>);
        }
    } catch {
        // Ignore malformed JSON here; the editor already surfaces syntax errors.
    }

    return {};
}

function setNestedValue(target: Record<string, unknown>, path: string[], value: unknown) {
    let current: Record<string, unknown> = target;
    for (let index = 0; index < path.length - 1; index += 1) {
        const key = path[index];
        const next = current[key];
        if (!next || typeof next !== 'object' || Array.isArray(next)) {
            current[key] = {};
        }
        current = current[key] as Record<string, unknown>;
    }
    current[path[path.length - 1]] = value;
}

function deleteNestedValue(target: Record<string, unknown>, path: string[]) {
    const parents: Array<Record<string, unknown>> = [];
    let current: Record<string, unknown> = target;

    for (let index = 0; index < path.length - 1; index += 1) {
        const key = path[index];
        const next = current[key];
        if (!next || typeof next !== 'object' || Array.isArray(next)) {
            return;
        }
        parents.push(current);
        current = next as Record<string, unknown>;
    }

    const leafKey = path[path.length - 1];
    if (!(leafKey in current)) {
        return;
    }
    delete current[leafKey];

    for (let index = path.length - 2; index >= 0; index -= 1) {
        if (Object.keys(current).length > 0) {
            return;
        }
        const parent = parents[index];
        const key = path[index];
        delete parent[key];
        current = parent;
    }
}

function formatCpu(value: number): string {
    return Number.isInteger(value) ? `${value}` : `${value}`;
}

function formatGi(value: number): string {
    return Number.isInteger(value) ? `${value}Gi` : `${value}Gi`;
}

function pruneUndefined(value: unknown): unknown {
    if (Array.isArray(value)) {
        return value.map((item) => pruneUndefined(item));
    }

    if (value && typeof value === 'object') {
        const next = Object.fromEntries(
            Object.entries(value as Record<string, unknown>)
                .filter(([, child]) => child !== undefined)
                .map(([key, child]) => [key, pruneUndefined(child)]),
        );
        return next;
    }

    return value;
}

export function buildResolvedInstanceSizePreview(values: InstanceSizeResolvedPreviewValues): string {
    const preview = parseSpecText(values.spec_text);

    if (typeof values.cpu_cores === 'number') {
        setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'cpu', 'cores'], values.cpu_cores);
        setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'resources', 'limits', 'cpu'], formatCpu(values.cpu_cores));

        if (values.dedicated_cpu || !values.cpu_overcommit_enabled) {
            setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'resources', 'requests', 'cpu'], formatCpu(values.cpu_cores));
        } else if (typeof values.cpu_request === 'number') {
            setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'resources', 'requests', 'cpu'], formatCpu(values.cpu_request));
        } else {
            deleteNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'resources', 'requests', 'cpu']);
        }
    }

    if (typeof values.memory_gi === 'number') {
        setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'memory', 'guest'], formatGi(values.memory_gi));
        setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'resources', 'limits', 'memory'], formatGi(values.memory_gi));

        if (!values.memory_overcommit_enabled) {
            setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'resources', 'requests', 'memory'], formatGi(values.memory_gi));
        } else if (typeof values.memory_request_gi === 'number') {
            setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'resources', 'requests', 'memory'], formatGi(values.memory_request_gi));
        } else {
            deleteNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'resources', 'requests', 'memory']);
        }
    }

    if (typeof values.dedicated_cpu === 'boolean') {
        setNestedValue(preview, ['spec', 'template', 'spec', 'domain', 'cpu', 'dedicatedCpuPlacement'], values.dedicated_cpu);
    }

    const platformSection = pruneUndefined({
        catalog_scope: values.catalog_scope,
        disk_gb: values.disk_gb,
        requires_sriov: values.requires_sriov,
        root_volume_mode_intent: values.root_volume_mode_intent,
        dv_access_modes: values.dv_access_modes,
        dv_volume_mode: values.dv_volume_mode,
        enabled: values.enabled,
    }) as Record<string, unknown>;

    const result = pruneUndefined({
        ...preview,
        ...(Object.keys(platformSection).length > 0 ? { _platform: platformSection } : {}),
    });

    return JSON.stringify(result, null, 2);
}
