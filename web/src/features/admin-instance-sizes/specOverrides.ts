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
 * matching nested branches removed; ancestors emptied **by this unset** are
 * pruned along the unset path only.
 *
 * Pruning semantics align with Lodash `_.unset` (which by design retains an
 * empty parent object after removing a leaf, e.g. `_.unset({a:[{b:{c:7}}]},
 * 'a[0].b.c')` -> `{a:[{b:{}}]}`) and never touch sibling branches. KubeVirt
 * relies on "marker" empty objects such as `livenessProbe.guestAgentPing: {}`,
 * `devices.rng: {}`, `interfaces[*].bridge: {}`, `networks[*].pod: {}` and
 * `clock.utc: {}` as legitimate leaf values (see the `// Empty map = leaf
 * value` invariant in internal/provider/vm_renderer.go). A naive whole-tree
 * prune would silently strip those markers and the KubeVirt admission webhook
 * would reject the resulting VM spec ("either ...livenessProbe.tcpSocket,
 * .exec or .httpGet must be set"), so cleanup is restricted to the ADR-0036
 * ancestor chain only.
 */
export function stripIndexedSpecOverridePaths(
    spec: Record<string, unknown> | undefined,
): Record<string, unknown> | undefined {
    if (!spec) {
        return spec;
    }
    // Deep clone before mutating: `unsetAndPruneAncestors` calls `delete` on
    // descendants, so a shallow `{ ...spec }` would leak writes into the
    // caller's tree. Today both call sites in `useAdminInstanceSizesController`
    // happen to feed us a normalized (already cloned) tree, but this is an
    // exported public API and must stay side-effect free regardless of caller.
    // `cloneSpecOverrides` uses the structuredClone deep-copy best practice
    // (MDN; falls back to JSON for older runtimes), which is safe here because
    // spec_overrides is by contract pure JSON.
    const cleaned = cloneSpecOverrides(spec);
    for (const path of [...INDEXED_SPEC_OVERRIDE_PATHS, ...LEGACY_INDEXED_SPEC_OVERRIDE_PATHS]) {
        delete cleaned[path];
        unsetAndPruneAncestors(cleaned, path);
    }
    return cleaned;
}

/**
 * Removes the leaf at `path` and bubbles up to prune ancestors that became
 * empty **as a direct result of this unset**. Sibling branches are never
 * inspected, so KubeVirt marker empty objects (e.g. `guestAgentPing: {}`,
 * `rng: {}`, `bridge: {}`, `pod: {}`) survive untouched.
 *
 * Aligns with Lodash `_.unset` (path-targeted, retains empty parents) and
 * adds one ADR-0036 boundary-cleanup rule: when the leaf removal genuinely
 * empties the parent chain, those empty ancestors are pruned upward until
 * either a non-empty ancestor is reached or the tree root is hit.
 *
 * The leaf-exists guard is required: an absent leaf cannot have been
 * "emptied by this unset", so we must not prune ancestors that were already
 * empty before the call (they encode their own KubeVirt marker semantics).
 */
function unsetAndPruneAncestors(
    target: Record<string, unknown>,
    path: string,
): void {
    const segments = path.split('.').filter(Boolean);
    if (segments.length === 0) {
        return;
    }
    const ancestors: Array<{ parent: Record<string, unknown>; key: string }> = [];
    let current: Record<string, unknown> = target;
    for (let index = 0; index < segments.length - 1; index += 1) {
        const next = current[segments[index]];
        if (!isRecord(next)) {
            return;
        }
        ancestors.push({ parent: current, key: segments[index] });
        current = next;
    }
    const leafKey = segments[segments.length - 1];
    if (!Object.prototype.hasOwnProperty.call(current, leafKey)) {
        return;
    }
    delete current[leafKey];
    for (let index = ancestors.length - 1; index >= 0; index -= 1) {
        const { parent, key } = ancestors[index];
        const value = parent[key];
        if (isRecord(value) && Object.keys(value).length === 0) {
            delete parent[key];
        } else {
            break;
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
