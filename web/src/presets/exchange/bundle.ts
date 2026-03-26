import { dump, load } from 'js-yaml';

import type {
    InstanceSizePresetCatalogItem,
    PresetCatalogSourceType,
    TemplatePresetCatalogItem,
} from '@/presets/catalog';

import {
    presetCatalogBundleSchema,
    type InstanceSizePresetBundleItem,
    type PresetCatalogBundle,
    type TemplatePresetBundleItem,
} from './schema';

interface BuildPresetCatalogBundleArgs<TTemplateValues, TInstanceSizeValues> {
    name: string;
    sourceType: PresetCatalogSourceType;
    templateItems?: Array<TemplatePresetCatalogItem<TTemplateValues>>;
    instanceSizeItems?: Array<InstanceSizePresetCatalogItem<TInstanceSizeValues>>;
    now?: Date;
}

function toTemplateBundleItem<TValues>(
    item: TemplatePresetCatalogItem<TValues>,
): TemplatePresetBundleItem {
    return {
        key: item.key,
        label: item.labelKey,
        description: item.descriptionKey,
        source_type: item.sourceType,
        verification_level: item.verificationLevel,
        os_family: item.osFamily,
        os_version: item.osVersion,
        values: item.values as Record<string, unknown>,
    };
}

function toInstanceSizeBundleItem<TValues>(
    item: InstanceSizePresetCatalogItem<TValues>,
): InstanceSizePresetBundleItem {
    return {
        key: item.key,
        label: item.labelKey,
        description: item.descriptionKey,
        source_type: item.sourceType,
        verification_level: item.verificationLevel,
        values: item.values as Record<string, unknown>,
    };
}

export function buildPresetCatalogBundle<TTemplateValues, TInstanceSizeValues>({
    name,
    sourceType,
    templateItems = [],
    instanceSizeItems = [],
    now = new Date(),
}: BuildPresetCatalogBundleArgs<TTemplateValues, TInstanceSizeValues>): PresetCatalogBundle {
    return {
        apiVersion: 'shepherd.io/v1alpha1',
        kind: 'PresetCatalogBundle',
        metadata: {
            name,
            source_type: sourceType,
            exported_at: now.toISOString(),
        },
        items: {
            templates: templateItems.map((item) => toTemplateBundleItem(item)),
            instance_sizes: instanceSizeItems.map((item) => toInstanceSizeBundleItem(item)),
        },
    };
}

export function serializePresetCatalogBundleToYAML(bundle: PresetCatalogBundle): string {
    return dump(bundle, {
        noRefs: true,
        lineWidth: 120,
        sortKeys: false,
    });
}

export function parsePresetCatalogBundleYAML(yamlText: string): PresetCatalogBundle {
    const parsed = load(yamlText);
    return presetCatalogBundleSchema.parse(parsed);
}
