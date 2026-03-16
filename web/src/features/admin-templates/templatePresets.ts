import {
    CURATED_TEMPLATE_PRESET_ITEMS,
    curatedLinuxCloudInitExample,
    curatedWindowsCloudInitExample,
    OFFICIAL_TEMPLATE_PRESET_ITEMS,
    type PresetCatalogSourceType,
    type CuratedTemplatePresetKey,
    type OfficialTemplatePresetKey,
    type TemplatePresetFormValues,
} from '@/presets/catalog';

export type TemplatePresetKey = CuratedTemplatePresetKey | OfficialTemplatePresetKey;

const templatePresetItems = [
    ...OFFICIAL_TEMPLATE_PRESET_ITEMS,
    ...CURATED_TEMPLATE_PRESET_ITEMS,
] as const;

const templatePresetItemByKey = Object.fromEntries(
    templatePresetItems.map((item) => [item.key, item]),
) as Record<TemplatePresetKey, (typeof templatePresetItems)[number]>;

export const TEMPLATE_PRESET_ORDER: TemplatePresetKey[] = [
    ...OFFICIAL_TEMPLATE_PRESET_ITEMS.map((item) => item.key as OfficialTemplatePresetKey),
    ...CURATED_TEMPLATE_PRESET_ITEMS.map((item) => item.key as CuratedTemplatePresetKey),
];

export const TEMPLATE_OS_FAMILY_OPTIONS = [
    { labelKey: 'templates.os_family_linux', value: 'linux' as const },
    { labelKey: 'templates.os_family_windows', value: 'windows' as const },
];

type TemplateCatalogScope = 'test' | 'prod' | 'all' | 'unclassified';

const TEMPLATE_PRESET_SCOPE_ORDER: TemplateCatalogScope[] = ['test', 'prod', 'all', 'unclassified'];

const TEMPLATE_PRESET_SCOPE_LABEL_KEYS: Record<TemplateCatalogScope, string> = {
    test: 'templates.scope_test',
    prod: 'templates.scope_prod',
    all: 'templates.scope_all',
    unclassified: 'templates.scope_unclassified',
};

const TEMPLATE_PRESET_GROUP_META: Record<
    Extract<PresetCatalogSourceType, 'official' | 'curated'>,
    { titleKey: string; descriptionKey: string }
> = {
    official: {
        titleKey: 'catalog.group_recommended',
        descriptionKey: 'templates.recommended_group_description',
    },
    curated: {
        titleKey: 'catalog.group_customized',
        descriptionKey: 'templates.customized_group_description',
    },
};

function groupTemplatePresetItemsByScope(
    items: typeof templatePresetItems,
) {
    return TEMPLATE_PRESET_SCOPE_ORDER
        .map((scope) => {
            const scopedItems = items.filter((item) => (item.values.catalog_scope ?? 'unclassified') === scope);
            if (scopedItems.length === 0) {
                return null;
            }
            return {
                scope,
                titleKey: TEMPLATE_PRESET_SCOPE_LABEL_KEYS[scope],
                items: scopedItems,
            };
        })
        .filter((group): group is NonNullable<typeof group> => group !== null);
}

export function buildTemplatePresetValues(key: TemplatePresetKey): TemplatePresetFormValues {
    return { ...templatePresetItemByKey[key].values };
}

export function getTemplatePresetGroups() {
    return [
        {
            sourceType: 'official' as const,
            ...TEMPLATE_PRESET_GROUP_META.official,
            scopeGroups: groupTemplatePresetItemsByScope(OFFICIAL_TEMPLATE_PRESET_ITEMS),
        },
        {
            sourceType: 'curated' as const,
            ...TEMPLATE_PRESET_GROUP_META.curated,
            scopeGroups: groupTemplatePresetItemsByScope(CURATED_TEMPLATE_PRESET_ITEMS),
        },
    ];
}

export function getTemplateOSVersionSuggestions(osFamily?: string): string[] {
    const wanted = (osFamily ?? '').trim().toLowerCase();
    return Array.from(
        new Set(
            templatePresetItems
                .filter((preset) => wanted === '' || preset.osFamily === wanted)
                .map((preset) => preset.osVersion),
        ),
    );
}

export function getTemplatePVCNamespaceSuggestions(): string[] {
    return Array.from(
        new Set(
            templatePresetItems
                .map((preset) => preset.pvcNamespace)
                .filter((value): value is string => typeof value === 'string' && value.trim() !== ''),
        ),
    );
}

export function getTemplatePVCNameSuggestions(osFamily?: string): string[] {
    const wanted = (osFamily ?? '').trim().toLowerCase();
    return Array.from(
        new Set(
            templatePresetItems
                .filter((preset) => wanted === '' || preset.osFamily === wanted)
                .map((preset) => preset.pvcName)
                .filter((value): value is string => typeof value === 'string' && value.trim() !== ''),
        ),
    );
}

export function getTemplateImageURLSuggestions(osFamily?: string): string[] {
    const wanted = (osFamily ?? '').trim().toLowerCase();
    return Array.from(
        new Set(
            templatePresetItems
                .filter((preset) => wanted === '' || preset.osFamily === wanted)
                .flatMap((preset) => preset.imageUrlExamples),
        ),
    );
}

export function getTemplateCloudInitExample(osFamily: 'linux' | 'windows'): string {
    return osFamily === 'windows' ? curatedWindowsCloudInitExample : curatedLinuxCloudInitExample;
}
