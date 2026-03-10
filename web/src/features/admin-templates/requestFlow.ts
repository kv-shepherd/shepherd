import type { Template } from './types';

export type TemplateRequestFlowStatus =
    | 'self_service'
    | 'admin_only_source'
    | 'hidden_unclassified'
    | 'disabled'
    | 'unsupported_source';

function normalizeTemplateSourceType(template: Pick<Template, 'source_type' | 'image_url' | 'pvc_name'>): string {
    if (template.source_type) {
        return template.source_type;
    }
    if (template.pvc_name) {
        return 'cdi_pvc_clone';
    }
    if (template.image_url) {
        return 'containerdisk';
    }
    return '';
}

export function getTemplateRequestFlowStatus(
    template: Pick<Template, 'enabled' | 'catalog_scope' | 'source_type' | 'image_url' | 'pvc_name'>,
): TemplateRequestFlowStatus {
    if (template.enabled === false) {
        return 'disabled';
    }

    const sourceType = normalizeTemplateSourceType(template);
    if (sourceType === 'containerdisk') {
        return 'admin_only_source';
    }
    if (sourceType !== 'cdi_image_import' && sourceType !== 'cdi_pvc_clone') {
        return 'unsupported_source';
    }

    const scope = (template.catalog_scope ?? 'unclassified').toLowerCase();
    if (scope === 'unclassified') {
        return 'hidden_unclassified';
    }

    return 'self_service';
}
