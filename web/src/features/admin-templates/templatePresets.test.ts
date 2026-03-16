import { describe, expect, it } from 'vitest';

import {
    buildTemplatePresetValues,
    getTemplateCloudInitExample,
    getTemplateImageURLSuggestions,
    getTemplateOSVersionSuggestions,
    getTemplatePresetGroups,
    getTemplatePVCNameSuggestions,
    getTemplatePVCNamespaceSuggestions,
} from './templatePresets';

describe('templatePresets', () => {
    it('builds the linux prod preset from the kubevirt template defaults', () => {
        expect(buildTemplatePresetValues('linux-prod')).toMatchObject({
            os_family: 'linux',
            os_version: 'Kylin V10',
            catalog_scope: 'prod',
            source_type: 'cdi_pvc_clone',
            pvc_namespace: 'vm-muban',
            pvc_name: 'kylinv10-image',
            enabled: true,
        });
    });

    it('provides os-aware suggestions instead of empty inputs only', () => {
        expect(getTemplateOSVersionSuggestions('linux')).toContain('openEuler 22.03');
        expect(getTemplateOSVersionSuggestions('linux')).toContain('Fedora');
        expect(getTemplateOSVersionSuggestions('windows')).toContain('Windows Server 2022');
        expect(getTemplatePVCNameSuggestions('windows')).toContain('win2022-image');
        expect(getTemplatePVCNamespaceSuggestions()).toContain('vm-muban');
        expect(getTemplateImageURLSuggestions('linux')).not.toHaveLength(0);
    });

    it('returns cloud-init examples for both linux and windows', () => {
        expect(getTemplateCloudInitExample('linux')).toContain('#cloud-config');
        expect(getTemplateCloudInitExample('windows')).toContain('#ps1_sysnative');
    });

    it('exposes curated and official preset groups for catalog-style picking', () => {
        const groups = getTemplatePresetGroups();
        expect(groups.map((group) => group.sourceType)).toEqual(['official', 'curated']);
        expect(groups[0]?.scopeGroups.map((group) => group.scope)).toEqual(['test']);
        expect(groups[1]?.scopeGroups.map((group) => group.scope)).toEqual(['test', 'prod']);
        expect(groups[0]?.scopeGroups[0]?.items.some((item) => item.key === 'official-fedora-eval')).toBe(true);
    });

    it('uses CDI image import for official starter presets', () => {
        expect(buildTemplatePresetValues('official-fedora-eval')).toMatchObject({
            source_type: 'cdi_image_import',
            image_url: 'docker://quay.io/containerdisks/fedora:latest',
        });
        expect(buildTemplatePresetValues('official-ubuntu-eval')).toMatchObject({
            source_type: 'cdi_image_import',
            image_url: 'docker://quay.io/containerdisks/ubuntu:latest',
        });
    });
});
