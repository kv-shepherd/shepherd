import { describe, expect, it } from 'vitest';

import { OFFICIAL_TEMPLATE_PRESET_ITEMS } from './officialTemplateCatalog';

describe('official template catalog internal audit', () => {
    it.each(OFFICIAL_TEMPLATE_PRESET_ITEMS)(
        'keeps template preset $key free of instance-size-only hardware overrides',
        ({ sourceType, verificationLevel, values }) => {
            const serialized = JSON.stringify(values);

            expect(sourceType).toBe('official');
            expect(verificationLevel).toBe('verified');
            expect(serialized).not.toContain('autoattachGraphicsDevice');
            expect(serialized).not.toContain('spec.template.spec.domain.devices');
        },
    );
});
