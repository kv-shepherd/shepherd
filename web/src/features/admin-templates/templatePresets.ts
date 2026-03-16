import {
    CURATED_TEMPLATE_PRESET_ITEMS,
    curatedLinuxCloudInitExample,
    curatedWindowsCloudInitExample,
    OFFICIAL_TEMPLATE_PRESET_ITEMS,
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

export function buildTemplatePresetValues(key: TemplatePresetKey): TemplatePresetFormValues {
    return { ...templatePresetItemByKey[key].values };
}

export function getTemplatePresetGroups() {
    return [
        {
            sourceType: 'official' as const,
            titleKey: 'catalog.source_official',
            descriptionKey: 'templates.official_group_description',
            items: OFFICIAL_TEMPLATE_PRESET_ITEMS,
        },
        {
            sourceType: 'curated' as const,
            titleKey: 'catalog.source_curated',
            descriptionKey: 'templates.curated_group_description',
            items: CURATED_TEMPLATE_PRESET_ITEMS,
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
