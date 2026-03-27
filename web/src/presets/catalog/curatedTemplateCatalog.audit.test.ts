import { describe, expect, it } from 'vitest';

import {
    CURATED_TEMPLATE_PRESET_ITEMS,
    curatedLinuxCloudInitExample,
    curatedWindowsCloudInitExample,
} from './curatedCatalog';

describe('curated template catalog internal audit', () => {
    it.each(CURATED_TEMPLATE_PRESET_ITEMS)(
        'keeps template preset $key internally consistent',
        ({ key, sourceType, verificationLevel, osFamily, osVersion, pvcNamespace, pvcName, imageUrlExamples, values }) => {
            const serialized = JSON.stringify(values);

            expect(sourceType).toBe('curated');
            expect(verificationLevel).toBe('verified');

            expect(values.source_type).toBe('cdi_pvc_clone');
            expect(values.pvc_namespace).toBe(pvcNamespace);
            expect(values.pvc_name).toBe(pvcName);
            expect(values.os_family).toBe(osFamily);
            expect(values.os_version).toBe(osVersion);
            expect(values.catalog_scope).toBe(key.includes('test') ? 'test' : 'prod');
            expect(values.image_url).toBeUndefined();
            expect(values.enabled).toBe(true);
            expect(serialized).not.toContain('autoattachGraphicsDevice');
            expect(serialized).not.toContain('spec.template.spec.domain.devices');

            expect(values.pvc_namespace).toBe('vm-muban');
            expect(imageUrlExamples.length).toBeGreaterThan(0);
            expect(imageUrlExamples.some((item) => item.startsWith('docker://') || item.startsWith('https://'))).toBe(true);

            if (key.startsWith('linux')) {
                expect(values.os_family).toBe('linux');
                expect(values.cloud_init).toBe(curatedLinuxCloudInitExample);
                expect(values.cloud_init?.startsWith('#cloud-config')).toBe(true);
            } else {
                expect(values.os_family).toBe('windows');
                expect(values.cloud_init).toBe(curatedWindowsCloudInitExample);
                expect(values.cloud_init?.startsWith('#ps1_sysnative')).toBe(true);
            }
        },
    );
});
