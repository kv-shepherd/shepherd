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

/**
 * Spec paths that are owned by the InstanceSize indexed columns (cpu_cores,
 * memory_gi, dedicated_cpu, cpu_request, memory_request_gi). They must NEVER
 * appear in `spec_overrides`: per ADR-0018 §4 the indexed columns are the
 * single source of truth for these fields, and the backend ADR-0036 guard
 * (HasDedicatedCPUInSpecOverrides + DetectSpecOverridesConflicts in
 * internal/service/instancesize_validator.go) rejects or warns whenever
 * the JSONB override contradicts the indexed column.
 *
 * The list is shared with DynamicSchemaForm via `recognizedExcludedPaths`
 * (see AdminInstanceSizesContent.tsx) so that the dynamic form UI hides
 * these paths, and reused by stripIndexedSpecOverridePaths() below to keep
 * spec_text / spec_overrides clean across every data boundary (preset
 * application, DB hydration, raw JSON edits, API submission).
 *
 * Both the canonical `spec.template.spec.domain.*` form and the legacy
 * `spec.domain.*` form are covered: legacy entries are migrated to canonical
 * form by normalizeInstanceSizeSpecOverrides, but the canonical-only stripping
 * runs *after* that migration so legacy keys are also removed transitively.
 */
export const INDEXED_SPEC_OVERRIDE_PATHS = [
    `${CANONICAL_DOMAIN_PATH}.cpu.cores`,
    `${CANONICAL_DOMAIN_PATH}.cpu.dedicatedCpuPlacement`,
    `${CANONICAL_DOMAIN_PATH}.memory.guest`,
    `${CANONICAL_DOMAIN_PATH}.resources.limits.cpu`,
    `${CANONICAL_DOMAIN_PATH}.resources.limits.memory`,
    `${CANONICAL_DOMAIN_PATH}.resources.requests.cpu`,
    `${CANONICAL_DOMAIN_PATH}.resources.requests.memory`,
] as const;

const LEGACY_INDEXED_SPEC_OVERRIDE_PATHS = INDEXED_SPEC_OVERRIDE_PATHS.map(
    (path) => `${LEGACY_DOMAIN_PREFIX}${path.slice(`${CANONICAL_DOMAIN_PATH}.`.length)}`,
);

/**
 * Removes every InstanceSize-indexed path from a spec_overrides map so the
 * indexed columns remain the single source of truth.
 *
 * This is the symmetrical counterpart to the DynamicSchemaForm UI exclusion:
 * the form already hides these paths, so they must not slip through preset
 * payloads, legacy DB rows, or hand-edited raw JSON. Centralising the rule
 * here mirrors the Ant Design `Form.Aggregate` bridge pattern
 * (https://ant.design/docs/blog/form-names) where derived/aggregate state is
 * normalised at the data boundary instead of being kept in sync via runtime
 * value propagation, avoiding double-source-of-truth races.
 *
 * Returns a shallow clone with both the flat dot-notation keys and the
 * matching nested branches removed; orphan empty parents are pruned so the
 * resulting spec_overrides is canonical and round-trippable.
 */
export function stripIndexedSpecOverridePaths(
    spec: Record<string, unknown> | undefined,
): Record<string, unknown> | undefined {
    if (!spec) {
        return spec;
    }
    const cleaned: Record<string, unknown> = { ...spec };
    for (const path of [...INDEXED_SPEC_OVERRIDE_PATHS, ...LEGACY_INDEXED_SPEC_OVERRIDE_PATHS]) {
        delete cleaned[path];
        deleteNestedValue(cleaned, path);
    }
    pruneEmptyBranches(cleaned);
    return cleaned;
}

function pruneEmptyBranches(target: Record<string, unknown>) {
    for (const [key, value] of Object.entries(target)) {
        if (!isRecord(value)) {
            continue;
        }
        pruneEmptyBranches(value);
        if (Object.keys(value).length === 0) {
            delete target[key];
        }
    }
}

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
