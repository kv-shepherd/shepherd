import { describe, expect, it } from 'vitest';

import { CURATED_INSTANCE_SIZE_PRESET_ITEMS } from '@/presets/catalog/curatedCatalog';

import { stripIndexedSpecOverridePaths } from './specOverrides';

/**
 * Pulls a parsed `spec_text` JSON tree from a curated InstanceSize preset.
 *
 * The curated catalog persists `spec_text` as a stringified KubeVirt VM spec,
 * which is the canonical input shape that `stripIndexedSpecOverridePaths`
 * receives at the form -> API data boundary in
 * `useAdminInstanceSizesController.formToPayload`.
 */
function getPresetSpec(key: string): Record<string, unknown> {
    const preset = CURATED_INSTANCE_SIZE_PRESET_ITEMS.find((item) => item.key === key);
    if (!preset) {
        throw new Error(`curated preset "${key}" not found`);
    }
    return JSON.parse(preset.values.spec_text) as Record<string, unknown>;
}

describe('stripIndexedSpecOverridePaths', () => {
    /**
     * a. Core mixed-tree case: prove pruning bubbles only along the unset path
     *    and never inspects sibling branches. The "cpu pruned + devices.rng
     *    untouched" pair is the golden combination — it is the most direct
     *    evidence that the cleanup runs along `domain.cpu.*` only.
     */
    it('prunes only along the unset path and keeps sibling KubeVirt marker empty objects intact', () => {
        const spec = {
            spec: {
                template: {
                    spec: {
                        domain: {
                            cpu: {
                                cores: 4,
                                dedicatedCpuPlacement: true,
                            },
                            devices: {
                                rng: {},
                                interfaces: [
                                    { bridge: {}, name: 'default', model: 'virtio' },
                                ],
                            },
                        },
                        livenessProbe: {
                            guestAgentPing: {},
                            initialDelaySeconds: 120,
                        },
                        networks: [{ name: 'default', pod: {} }],
                    },
                },
            },
        };

        const cleaned = stripIndexedSpecOverridePaths(spec) as Record<string, unknown>;

        // (1) indexed-column fields are removed and the now-empty `cpu` parent
        //     is pruned along the unset path; `domain` retains its other
        //     children (devices) so it is *not* pruned.
        expect(cleaned).not.toHaveProperty('spec.template.spec.domain.cpu');
        expect(cleaned).toHaveProperty('spec.template.spec.domain.devices');

        // (2) every sibling marker empty object survives untouched.
        expect(cleaned).toHaveProperty('spec.template.spec.domain.devices.rng', {});
        expect(cleaned).toHaveProperty(
            'spec.template.spec.domain.devices.interfaces.0.bridge',
            {},
        );
        expect(cleaned).toHaveProperty('spec.template.spec.livenessProbe.guestAgentPing', {});
        expect(cleaned).toHaveProperty('spec.template.spec.networks.0.pod', {});
    });

    /**
     * b. When every descendant of an ancestor chain is owned by indexed
     *    columns, the entire chain collapses upward to the root.
     */
    it('prunes the full ancestor chain when every leaf is owned by indexed columns', () => {
        const spec = {
            spec: {
                template: {
                    spec: {
                        domain: {
                            cpu: { cores: 4, dedicatedCpuPlacement: true },
                        },
                    },
                },
            },
        };

        const cleaned = stripIndexedSpecOverridePaths(spec);

        expect(cleaned).toEqual({});
    });

    /**
     * c. Pruning must stop at the first non-empty ancestor — siblings that
     *    are unrelated to indexed columns continue to live in the tree.
     */
    it('keeps non-empty ancestors intact when only some leaves are pruned', () => {
        const spec = {
            spec: {
                template: {
                    spec: {
                        domain: {
                            cpu: { cores: 4, model: 'host-passthrough' },
                        },
                    },
                },
            },
        };

        const cleaned = stripIndexedSpecOverridePaths(spec) as Record<string, unknown>;

        expect(cleaned).toHaveProperty('spec.template.spec.domain.cpu', {
            model: 'host-passthrough',
        });
    });

    /**
     * d. Top-level dot-notation keys (legacy raw-JSON shape) keep being
     *    deleted via the existing `delete cleaned[path]` branch.
     */
    it('removes flat dot-notation top-level keys without touching unrelated entries', () => {
        const spec = {
            'spec.template.spec.domain.cpu.cores': 4,
            'spec.template.spec.domain.cpu.dedicatedCpuPlacement': true,
            'spec.template.spec.domain.devices.rng': {},
        };

        const cleaned = stripIndexedSpecOverridePaths(spec) as Record<string, unknown>;

        expect(cleaned['spec.template.spec.domain.cpu.cores']).toBeUndefined();
        expect(cleaned['spec.template.spec.domain.cpu.dedicatedCpuPlacement']).toBeUndefined();
        expect(cleaned['spec.template.spec.domain.devices.rng']).toEqual({});
    });

    /**
     * d2. Legacy nested form (`spec.domain.*`) is also stripped along the
     *     LEGACY_INDEXED_SPEC_OVERRIDE_PATHS list. Without this regression
     *     a future refactor of the legacy-prefix derivation could silently
     *     re-introduce phantom indexed-column values from old DB rows.
     */
    it('strips legacy nested spec.domain.* paths and prunes their ancestors', () => {
        const spec = {
            spec: {
                domain: {
                    cpu: { cores: 4, dedicatedCpuPlacement: true },
                    devices: { rng: {} },
                },
            },
        };

        const cleaned = stripIndexedSpecOverridePaths(spec) as Record<string, unknown>;

        // Indexed-column leaves are removed and the now-empty `cpu` parent is
        // pruned along the unset path.
        expect(cleaned).not.toHaveProperty('spec.domain.cpu');
        // Sibling marker empty object on the legacy branch survives.
        expect(cleaned).toHaveProperty('spec.domain.devices.rng', {});
    });

    /**
     * d3. The function must stay side-effect free: the caller's tree is
     *     never mutated, even though the internal pruner calls `delete`
     *     on descendants. Guards against a regression where a future
     *     refactor reverts the deep clone to a shallow `{ ...spec }`.
     */
    it('does not mutate the input spec', () => {
        const spec = {
            spec: {
                template: {
                    spec: {
                        domain: {
                            cpu: { cores: 4, dedicatedCpuPlacement: true },
                            devices: { rng: {} },
                        },
                        livenessProbe: { guestAgentPing: {} },
                    },
                },
            },
        };
        const before = JSON.stringify(spec);

        stripIndexedSpecOverridePaths(spec);

        expect(JSON.stringify(spec)).toBe(before);
    });

    /**
     * e1. Precise regression for the production incident: the curated
     *     `linux-prod` preset must keep `livenessProbe.guestAgentPing: {}`
     *     after the boundary cleanse, otherwise the KubeVirt admission
     *     webhook rejects the dry-run with
     *     "either ...livenessProbe.tcpSocket, .exec or .httpGet must be set".
     */
    it('linux-prod preset retains livenessProbe.guestAgentPing as {} after strip', () => {
        const spec = getPresetSpec('linux-prod');

        const cleaned = stripIndexedSpecOverridePaths(spec) as Record<string, unknown>;

        expect(cleaned).toHaveProperty('spec.template.spec.livenessProbe.guestAgentPing', {});
    });

    /**
     * e2. The Windows curated preset has no livenessProbe, so we exercise
     *     the regression on its other KubeVirt marker empty objects:
     *     devices.rng, interfaces[0].bridge, networks[0].pod, plus the
     *     Windows-specific clock.utc and clock.timer.hyperv.
     */
    it('windows-prod preset retains every KubeVirt marker empty object after strip', () => {
        const spec = getPresetSpec('windows-prod');

        const cleaned = stripIndexedSpecOverridePaths(spec) as Record<string, unknown>;

        expect(cleaned).toHaveProperty('spec.template.spec.domain.devices.rng', {});
        expect(cleaned).toHaveProperty(
            'spec.template.spec.domain.devices.interfaces.0.bridge',
            {},
        );
        expect(cleaned).toHaveProperty('spec.template.spec.networks.0.pod', {});
        expect(cleaned).toHaveProperty('spec.template.spec.domain.clock.utc', {});
        expect(cleaned).toHaveProperty('spec.template.spec.domain.clock.timer.hyperv', {});
    });

    /**
     * f. Leaf-exists guard: an ancestor that was *already* empty before the
     *    strip call must not be pruned. Without the guard, the pruner would
     *    misattribute pre-existing emptiness to "this unset" and erase
     *    KubeVirt marker objects placed by the operator on purpose.
     */
    it('leaves ancestors that were already empty before the strip untouched', () => {
        const spec = {
            spec: {
                template: {
                    spec: {
                        domain: {
                            // `cpu` is already `{}` and `cpu.cores` does not exist;
                            // the guard must skip pruning so an operator's deliberate
                            // marker empty object survives.
                            cpu: {},
                        },
                    },
                },
            },
        };

        const cleaned = stripIndexedSpecOverridePaths(spec) as Record<string, unknown>;

        expect(cleaned).toHaveProperty('spec.template.spec.domain.cpu', {});
    });
});
