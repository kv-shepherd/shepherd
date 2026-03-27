import { describe, expect, it } from 'vitest';

import { OFFICIAL_INSTANCE_SIZE_PRESET_ITEMS } from './officialInstanceSizeCatalog';

function parseSpecText(specText: string): Record<string, unknown> {
    return JSON.parse(specText) as Record<string, unknown>;
}

function getAtPath(value: unknown, pathText: string): unknown {
    const parts = pathText.split('.');
    let current: unknown = value;
    for (const part of parts) {
        if (!current || typeof current !== 'object' || Array.isArray(current)) {
            return undefined;
        }
        current = (current as Record<string, unknown>)[part];
    }
    return current;
}

describe('official instance size catalog internal audit', () => {
    it.each(OFFICIAL_INSTANCE_SIZE_PRESET_ITEMS)(
        'keeps instance-size preset $key VNC-capable through instance size spec overrides',
        ({ sourceType, verificationLevel, values }) => {
            const parsed = parseSpecText(values.spec_text);

            expect(sourceType).toBe('official');
            expect(verificationLevel).toBe('verified');
            expect(values.enabled).toBe(true);
            expect(values.catalog_scope).toBe('all');
            expect(getAtPath(parsed, 'spec.template.spec.domain.devices.autoattachGraphicsDevice')).toBe(true);
        },
    );
});
