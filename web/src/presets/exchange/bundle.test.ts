import { describe, expect, it } from 'vitest';

import {
    CURATED_INSTANCE_SIZE_PRESET_ITEMS,
    CURATED_TEMPLATE_PRESET_ITEMS,
    OFFICIAL_TEMPLATE_PRESET_ITEMS,
} from '@/presets/catalog';

import {
    buildPresetCatalogBundle,
    parsePresetCatalogBundleYAML,
    serializePresetCatalogBundleToYAML,
} from './bundle';

describe('preset bundle exchange', () => {
    it('builds and parses a curated bundle with templates and instance sizes', () => {
        const bundle = buildPresetCatalogBundle({
            name: 'curated-catalog',
            sourceType: 'curated',
            templateItems: CURATED_TEMPLATE_PRESET_ITEMS,
            instanceSizeItems: CURATED_INSTANCE_SIZE_PRESET_ITEMS,
            now: new Date('2026-03-12T00:00:00Z'),
        });

        const yamlText = serializePresetCatalogBundleToYAML(bundle);
        const reparsed = parsePresetCatalogBundleYAML(yamlText);

        expect(reparsed.metadata.name).toBe('curated-catalog');
        expect(reparsed.items.templates).toHaveLength(CURATED_TEMPLATE_PRESET_ITEMS.length);
        expect(reparsed.items.instance_sizes).toHaveLength(CURATED_INSTANCE_SIZE_PRESET_ITEMS.length);
    });

    it('preserves official template starter items in yaml bundles', () => {
        const bundle = buildPresetCatalogBundle({
            name: 'official-template-catalog',
            sourceType: 'official',
            templateItems: OFFICIAL_TEMPLATE_PRESET_ITEMS,
            now: new Date('2026-03-12T00:00:00Z'),
        });

        const yamlText = serializePresetCatalogBundleToYAML(bundle);

        expect(yamlText).toContain('kind: PresetCatalogBundle');
        expect(yamlText).toContain('official-fedora-eval');
        expect(yamlText).toContain('quay.io/containerdisks/fedora:latest');
    });
});
