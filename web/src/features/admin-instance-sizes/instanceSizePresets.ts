import {
    CURATED_INSTANCE_SIZE_PRESET_ITEMS,
    OFFICIAL_INSTANCE_SIZE_PRESET_ITEMS,
    type PresetCatalogSourceType,
    type CuratedInstanceSizePresetFormValues,
    type CuratedInstanceSizePresetKey,
    type OfficialInstanceSizePresetKey,
} from '@/presets/catalog';

export type InstanceSizePresetKey = CuratedInstanceSizePresetKey | OfficialInstanceSizePresetKey;

export type InstanceSizePresetFormValues = CuratedInstanceSizePresetFormValues;

const instanceSizePresetItems = [
    ...OFFICIAL_INSTANCE_SIZE_PRESET_ITEMS,
    ...CURATED_INSTANCE_SIZE_PRESET_ITEMS,
] as const;

const instanceSizePresetItemByKey = Object.fromEntries(
    instanceSizePresetItems.map((item) => [item.key, item]),
) as Record<InstanceSizePresetKey, (typeof instanceSizePresetItems)[number]>;

export const INSTANCE_SIZE_PRESET_ORDER: InstanceSizePresetKey[] = instanceSizePresetItems.map(
    (item) => item.key as InstanceSizePresetKey,
);

type InstanceSizeCatalogScope = 'test' | 'prod' | 'all' | 'unclassified';

const INSTANCE_SIZE_PRESET_SCOPE_ORDER: InstanceSizeCatalogScope[] = ['test', 'prod', 'all', 'unclassified'];

const INSTANCE_SIZE_PRESET_SCOPE_LABEL_KEYS: Record<InstanceSizeCatalogScope, string> = {
    test: 'templates.scope_test',
    prod: 'templates.scope_prod',
    all: 'templates.scope_all',
    unclassified: 'templates.scope_unclassified',
};

const INSTANCE_SIZE_GROUP_META: Record<
    Extract<PresetCatalogSourceType, 'curated' | 'official'>,
    { titleKey: string; descriptionKey: string }
> = {
    official: {
        titleKey: 'catalog.group_recommended',
        descriptionKey: 'instanceSizes.recommended_group_description',
    },
    curated: {
        titleKey: 'catalog.group_customized',
        descriptionKey: 'instanceSizes.customized_group_description',
    },
};

export function buildInstanceSizePresetValues(key: InstanceSizePresetKey): InstanceSizePresetFormValues {
    return { ...instanceSizePresetItemByKey[key].values };
}

export function getInstanceSizePresetLabelKey(key: InstanceSizePresetKey): string {
    return instanceSizePresetItemByKey[key].labelKey;
}

export function getInstanceSizePresetGroups() {
    return [
        {
            sourceType: 'official' as const,
            ...INSTANCE_SIZE_GROUP_META.official,
            scopeGroups: INSTANCE_SIZE_PRESET_SCOPE_ORDER
                .map((scope) => {
                    const items = OFFICIAL_INSTANCE_SIZE_PRESET_ITEMS.filter(
                        (item) => (item.values.catalog_scope ?? 'unclassified') === scope,
                    );
                    if (items.length === 0) {
                        return null;
                    }
                    return {
                        scope,
                        titleKey: INSTANCE_SIZE_PRESET_SCOPE_LABEL_KEYS[scope],
                        items,
                    };
                })
                .filter((group): group is NonNullable<typeof group> => group !== null),
        },
        {
            sourceType: 'curated' as const,
            ...INSTANCE_SIZE_GROUP_META.curated,
            scopeGroups: INSTANCE_SIZE_PRESET_SCOPE_ORDER
                .map((scope) => {
                    const items = CURATED_INSTANCE_SIZE_PRESET_ITEMS.filter(
                        (item) => (item.values.catalog_scope ?? 'unclassified') === scope,
                    );
                    if (items.length === 0) {
                        return null;
                    }
                    return {
                        scope,
                        titleKey: INSTANCE_SIZE_PRESET_SCOPE_LABEL_KEYS[scope],
                        items,
                    };
                })
                .filter((group): group is NonNullable<typeof group> => group !== null),
        },
    ];
}
