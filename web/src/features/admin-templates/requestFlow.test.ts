import { describe, expect, it } from 'vitest';

import { getTemplateRequestFlowStatus } from './requestFlow';

describe('getTemplateRequestFlowStatus', () => {
    it('returns self_service for enabled persistent templates with classified scope', () => {
        expect(
            getTemplateRequestFlowStatus({
                enabled: true,
                source_type: 'cdi_image_import',
                catalog_scope: 'prod',
            }),
        ).toBe('self_service');
    });

    it('returns admin_only_source for containerdisk templates', () => {
        expect(
            getTemplateRequestFlowStatus({
                enabled: true,
                source_type: 'containerdisk',
                catalog_scope: 'test',
            }),
        ).toBe('admin_only_source');
    });

    it('returns hidden_unclassified for unclassified persistent templates', () => {
        expect(
            getTemplateRequestFlowStatus({
                enabled: true,
                source_type: 'cdi_pvc_clone',
                catalog_scope: 'unclassified',
            }),
        ).toBe('hidden_unclassified');
    });

    it('returns disabled before checking source or scope', () => {
        expect(
            getTemplateRequestFlowStatus({
                enabled: false,
                source_type: 'cdi_image_import',
                catalog_scope: 'prod',
            }),
        ).toBe('disabled');
    });

    it('falls back to pvc metadata for legacy clone rows without source_type', () => {
        expect(
            getTemplateRequestFlowStatus({
                enabled: true,
                pvc_name: 'golden-root',
                catalog_scope: 'test',
            }),
        ).toBe('self_service');
    });
});
