import type { components } from '@/types/api.gen';

export type Template = components['schemas']['Template'];
export type TemplateList = components['schemas']['TemplateList'];
export type TemplateCreateRequest = components['schemas']['TemplateCreateRequest'];
export type TemplateUpdateRequest = components['schemas']['TemplateUpdateRequest'];
export type TemplateSourceType = NonNullable<TemplateCreateRequest['source_type']>;
export type TemplateCatalogScope = NonNullable<TemplateCreateRequest['catalog_scope']>;

export const OS_COLOR_MAP: Record<string, string> = {
    linux: 'green',
    windows: 'blue',
    centos: 'orange',
    ubuntu: 'geekblue',
    debian: 'purple',
    rhel: 'red',
    fedora: 'cyan',
};
