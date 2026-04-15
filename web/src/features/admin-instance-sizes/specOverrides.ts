function isRecord(value: unknown): value is Record<string, unknown> {
    return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function cloneSpecOverrides(spec: Record<string, unknown> | undefined): Record<string, unknown> {
    if (!spec) {
        return {};
    }
    if (typeof structuredClone === 'function') {
        return structuredClone(spec) as Record<string, unknown>;
    }
    return JSON.parse(JSON.stringify(spec)) as Record<string, unknown>;
}

export function getSpecOverrideValue(spec: Record<string, unknown> | undefined, path: string): unknown {
    if (!spec || !path) {
        return undefined;
    }
    if (path in spec) {
        return spec[path];
    }
    const segments = path.split('.');
    let current: unknown = spec;
    for (const segment of segments) {
        if (!isRecord(current)) {
            return undefined;
        }
        current = current[segment];
    }
    return current;
}

export function setNestedValue(target: Record<string, unknown>, path: string, value: unknown) {
    const segments = path.split('.');
    let current: Record<string, unknown> = target;
    for (let index = 0; index < segments.length; index += 1) {
        const segment = segments[index];
        if (index === segments.length - 1) {
            current[segment] = value;
            return;
        }
        const next = current[segment];
        if (!isRecord(next)) {
            current[segment] = {};
        }
        current = current[segment] as Record<string, unknown>;
    }
}

function deleteNestedValue(target: Record<string, unknown>, path: string) {
    const segments = path.split('.');
    let current: Record<string, unknown> = target;
    for (let index = 0; index < segments.length - 1; index += 1) {
        const segment = segments[index];
        const next = current[segment];
        if (!isRecord(next)) {
            return;
        }
        current = next;
    }
    delete current[segments[segments.length - 1]];
}

function mergeIntoMissing(target: Record<string, unknown>, source: Record<string, unknown>) {
    for (const [key, value] of Object.entries(source)) {
        const existing = target[key];
        if (existing === undefined) {
            target[key] = cloneSpecOverrides({ value }).value;
            continue;
        }
        if (isRecord(existing) && isRecord(value)) {
            mergeIntoMissing(existing, value);
        }
    }
}

const LEGACY_DOMAIN_PREFIX = 'spec.domain.';
const LEGACY_DOMAIN_PATH = 'spec.domain';
const CANONICAL_DOMAIN_PATH = 'spec.template.spec.domain';

export const CANONICAL_DEDICATED_CPU_PATH = `${CANONICAL_DOMAIN_PATH}.cpu.dedicatedCpuPlacement` as const;

export function normalizeInstanceSizeSpecOverrides(
    spec: Record<string, unknown> | undefined,
): Record<string, unknown> {
    const normalized = cloneSpecOverrides(spec);

    const legacyDomain = getSpecOverrideValue(normalized, LEGACY_DOMAIN_PATH);
    if (isRecord(legacyDomain)) {
        const canonicalDomain = getSpecOverrideValue(normalized, CANONICAL_DOMAIN_PATH);
        if (isRecord(canonicalDomain)) {
            mergeIntoMissing(canonicalDomain, legacyDomain);
        } else {
            setNestedValue(normalized, CANONICAL_DOMAIN_PATH, cloneSpecOverrides(legacyDomain));
        }
        deleteNestedValue(normalized, LEGACY_DOMAIN_PATH);
    }

    for (const [path, value] of Object.entries(normalized)) {
        if (!path.startsWith(LEGACY_DOMAIN_PREFIX)) {
            continue;
        }
        const canonicalPath = `${CANONICAL_DOMAIN_PATH}.${path.slice(LEGACY_DOMAIN_PREFIX.length)}`;
        if (!(canonicalPath in normalized)) {
            normalized[canonicalPath] = value;
        }
        delete normalized[path];
    }

    return normalized;
}
