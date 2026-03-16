import {
    CURATED_INSTANCE_SIZE_PRESET_ITEMS,
    type CuratedInstanceSizePresetFormValues,
    type CuratedInstanceSizePresetKey,
} from '@/presets/catalog';

export type InstanceSizePresetKey = CuratedInstanceSizePresetKey;

export type InstanceSizePresetFormValues = CuratedInstanceSizePresetFormValues;

const instanceSizePresetItemByKey = Object.fromEntries(
    CURATED_INSTANCE_SIZE_PRESET_ITEMS.map((item) => [item.key, item]),
) as Record<InstanceSizePresetKey, (typeof CURATED_INSTANCE_SIZE_PRESET_ITEMS)[number]>;

export const INSTANCE_SIZE_PRESET_ORDER: InstanceSizePresetKey[] = CURATED_INSTANCE_SIZE_PRESET_ITEMS.map(
    (item) => item.key as InstanceSizePresetKey,
);

export function buildInstanceSizePresetValues(key: InstanceSizePresetKey): InstanceSizePresetFormValues {
    return { ...instanceSizePresetItemByKey[key].values };
}

export function getInstanceSizePresetLabelKey(key: InstanceSizePresetKey): string {
    return instanceSizePresetItemByKey[key].labelKey;
}
